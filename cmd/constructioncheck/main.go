// Command constructioncheck is the construction release check (ADR 0123): it
// fetches and verifies the pinned OGA files, runs the ungated detectors over
// them, joins the output to the committed ID-only judgment file and prints
// precision per construction and subtype against the shipping bar. It exits
// non-zero when a shipped unit is below the bar, a judged row flipped against
// its baseline, or a construction's hit count drifted more than 10%.
//
// Run it through `make construction-release-check` and paste the output into
// the PR of any construction detector version bump. It is not a CI step.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/constructions/releasecheck"
)

const dataDir = "internal/constructions/releasecheck/"

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// errFailed marks a completed check whose result is a failure.
var errFailed = errors.New("construction release check failed")

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("constructioncheck", flag.ContinueOnError)
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	cache := flags.String("cache", filepath.Join(home, ".cache", "mouseion", "oga-src"), "directory for the verified OGA files (never inside the repository)")
	baseline := flags.Bool("baseline", false, "write emitted_baseline and counts_baseline.tsv from the current output instead of checking")
	judgmentsPath := flags.String("judgments", dataDir+"judgments.tsv", "judgment file")
	countsPath := flags.String("research-counts", dataDir+"research_counts.tsv", "research corpus counts")
	baselinePath := flags.String("counts-baseline", dataDir+"counts_baseline.tsv", "baseline hit counts")
	if err := flags.Parse(args); err != nil {
		return err
	}

	if err := checkCache(*cache); err != nil {
		return err
	}

	definitions, err := classical.ParseDefinitions(classical.ManifestJSONL)
	if err != nil {
		return err
	}
	layers, err := classical.ParseLayers(classical.LayersJSON)
	if err != nil {
		return err
	}
	fetcher := &cachedFetcher{dir: *cache, layers: layers, client: &http.Client{Timeout: 5 * time.Minute}}
	detected, err := releasecheck.DetectCorpus(ctx, definitions, fetcher)
	if err != nil {
		return err
	}
	judgmentsData, err := os.ReadFile(*judgmentsPath)
	if err != nil {
		return err
	}
	judgments, err := releasecheck.ParseJudgments(bytes.NewReader(judgmentsData))
	if err != nil {
		return err
	}
	order := constructionOrder()
	if *baseline {
		return writeBaseline(*judgmentsPath, *baselinePath, judgments, detected, order)
	}
	counts, err := readCounts(*countsPath)
	if err != nil {
		return err
	}
	base, err := readBaseline(*baselinePath)
	if err != nil {
		return err
	}
	report, err := releasecheck.Evaluate(releasecheck.Input{
		Judgments: judgments, ResearchCount: counts, Baseline: base, Detected: detected,
		Gate: gateAdapter{constructions.ShippingGate}, Constructions: order,
		DetectorVersion: constructions.AGDTDetectorVersion,
	})
	if err != nil {
		return err
	}
	var header strings.Builder
	for _, name := range fetcher.downloaded {
		fmt.Fprintf(&header, "downloaded %s\n", name)
	}
	fmt.Fprintf(&header, "detector version: %d\n\n", constructions.AGDTDetectorVersion)
	if _, err := io.WriteString(out, header.String()); err != nil {
		return err
	}
	if err := report.Print(out); err != nil {
		return err
	}
	if len(report.Failures) > 0 {
		return errFailed
	}
	return nil
}

func constructionOrder() []string {
	order := make([]string, len(constructions.All))
	for i, id := range constructions.All {
		order[i] = string(id)
	}
	return order
}

// gateAdapter exposes the shipping gate with string ids.
type gateAdapter struct{ gate constructions.Gate }

func (g gateAdapter) Ships(id string) bool { return g.gate.Ships(constructions.ID(id)) }
func (g gateAdapter) ShipsLabel(id, subtype string) bool {
	return g.gate.ShipsLabel(constructions.ID(id), constructions.SubtypeID(subtype))
}

func readCounts(path string) (map[releasecheck.CountKey]int, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the counts file.
	if err != nil {
		return nil, err
	}
	return releasecheck.ParseResearchCounts(bytes.NewReader(data))
}

func readBaseline(path string) (releasecheck.Baseline, error) {
	data, err := os.ReadFile(path) //nolint:gosec // the operator names the baseline file.
	if err != nil {
		return releasecheck.Baseline{}, fmt.Errorf("%w (write it once with -baseline)", err)
	}
	return releasecheck.ParseBaseline(bytes.NewReader(data))
}

// writeBaseline records the current output as the baseline.
func writeBaseline(judgmentsPath, baselinePath string, judgments []releasecheck.Judgment, detected releasecheck.Detected, order []string) error {
	for i := range judgments {
		_, judgments[i].EmittedBaseline = detected.Emitted[judgments[i].Key()]
	}
	var judged strings.Builder
	if err := releasecheck.WriteJudgments(&judged, judgments); err != nil {
		return err
	}
	if err := os.WriteFile(judgmentsPath, []byte(judged.String()), 0o600); err != nil {
		return err
	}
	var counts strings.Builder
	base := releasecheck.Baseline{DetectorVersion: constructions.AGDTDetectorVersion, Hits: detected.Hits}
	if err := releasecheck.WriteBaseline(&counts, base, order); err != nil {
		return err
	}
	return os.WriteFile(baselinePath, []byte(counts.String()), 0o600)
}

// cachedFetcher serves a pinned file from the cache when it verifies and
// downloads it otherwise. Files are verified again by DetectCorpus.
type cachedFetcher struct {
	dir    string
	layers classical.Layers
	client *http.Client
	// downloaded names the files fetched over the network, in order.
	downloaded []string
}

func (f *cachedFetcher) Fetch(ctx context.Context, pin classical.SourcePin) (data []byte, err error) {
	path := filepath.Join(f.dir, filepath.Base(pin.FileName))
	//nolint:gosec // the cache directory is the operator's, not request input.
	if data, err := os.ReadFile(path); err == nil && releasecheck.Verify(data, pin.Size, pin.SHA256) == nil {
		return data, nil
	}
	layer, ok := f.layers.Layer(pin.Layer)
	if !ok {
		return nil, fmt.Errorf("unknown layer %q", pin.Layer)
	}
	source, err := sourceURL(layer.BaseURL, pin.FileName)
	if err != nil {
		return nil, err
	}
	f.downloaded = append(f.downloaded, pin.FileName)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, source, nil)
	if err != nil {
		return nil, err
	}
	response, err := f.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer func() { err = errors.Join(err, response.Body.Close()) }()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("fetch %s: status %d", pin.FileName, response.StatusCode)
	}
	data, err = io.ReadAll(io.LimitReader(response.Body, pin.Size+1))
	if err != nil {
		return nil, err
	}
	if err := releasecheck.Verify(data, pin.Size, pin.SHA256); err != nil {
		return nil, fmt.Errorf("%s: %w", pin.FileName, err)
	}
	if err := os.MkdirAll(f.dir, 0o750); err != nil {
		return nil, err
	}
	//nolint:gosec // the cache directory is the operator's, not request input.
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return nil, err
	}
	return data, nil
}

// sourceURL joins a layer base URL and a pinned file name.
func sourceURL(baseURL, fileName string) (string, error) {
	base, err := url.Parse(baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return "", errors.New("layer base URL is invalid")
	}
	if !strings.HasSuffix(base.Path, "/") {
		base.Path += "/"
	}
	reference, err := url.Parse(fileName)
	if err != nil {
		return "", err
	}
	return base.ResolveReference(reference).String(), nil
}

// repositoryRoot walks up from dir to the directory holding go.mod.
func repositoryRoot(dir string) (string, error) {
	for current := dir; ; {
		if _, err := os.Stat(filepath.Join(current, "go.mod")); err == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", errors.New("run constructioncheck from inside the repository (no go.mod found)")
		}
		current = parent
	}
}

// rejectCacheInside refuses a cache directory that resolves, after following
// symlinks, to the repository root or anywhere below it, so OGA files can
// never be written into the repository.
func rejectCacheInside(cache, root string) error {
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return err
	}
	resolved, err := resolveExisting(cache)
	if err != nil {
		return err
	}
	relative, err := filepath.Rel(resolvedRoot, resolved)
	if err != nil {
		return err
	}
	if relative == "." || (relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator))) {
		return fmt.Errorf("cache directory %s is inside the repository (%s); choose a path outside it so OGA files are never committed", cache, resolvedRoot)
	}
	return nil
}

// resolveExisting resolves symlinks in the longest existing prefix of path and
// appends the part that does not exist yet.
func resolveExisting(path string) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	rest := ""
	for current := absolute; ; {
		if resolved, err := filepath.EvalSymlinks(current); err == nil {
			return filepath.Join(resolved, rest), nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return absolute, nil
		}
		rest = filepath.Join(filepath.Base(current), rest)
		current = parent
	}
}

// checkCache refuses a cache directory inside the repository containing the
// working directory.
func checkCache(cache string) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	root, err := repositoryRoot(cwd)
	if err != nil {
		return err
	}
	return rejectCacheInside(cache, root)
}
