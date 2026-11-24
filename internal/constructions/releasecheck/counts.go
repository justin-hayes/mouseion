package releasecheck

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
)

// CountsHeader is the exact header of research_counts.tsv.
const CountsHeader = "construction\tstratum\tstratum_detail\tsubtype\tcount"

// CountKey names one research stratum.
type CountKey struct{ Construction, Stratum, Detail, Subtype string }

// ParseResearchCounts reads research_counts.tsv: the research corpus count of
// each stratum (results-tables.md of the #2615 re-measurement).
func ParseResearchCounts(r io.Reader) (map[CountKey]int, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() || scanner.Text() != CountsHeader {
		return nil, errors.New("releasecheck: research counts header is not the expected one")
	}
	out := map[CountKey]int{}
	for line := 2; scanner.Scan(); line++ {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 5 {
			return nil, fmt.Errorf("releasecheck: research counts line %d has %d fields, want 5", line, len(fields))
		}
		count, err := strconv.Atoi(fields[4])
		if err != nil || count < 0 {
			return nil, fmt.Errorf("releasecheck: research counts line %d count is not a number", line)
		}
		key := CountKey{fields[0], fields[1], fields[2], fields[3]}
		if _, dup := out[key]; dup {
			return nil, fmt.Errorf("releasecheck: research counts line %d repeats %v", line, key)
		}
		out[key] = count
	}
	return out, scanner.Err()
}

// BaselineHeader is the exact header of counts_baseline.tsv.
const BaselineHeader = "detector_version\tconstruction\thits"

// Baseline is the ungated hit count per construction at one detector version.
type Baseline struct {
	DetectorVersion int
	Hits            map[string]int
}

// ParseBaseline reads counts_baseline.tsv.
func ParseBaseline(r io.Reader) (Baseline, error) {
	scanner := bufio.NewScanner(r)
	if !scanner.Scan() || scanner.Text() != BaselineHeader {
		return Baseline{}, errors.New("releasecheck: counts baseline header is not the expected one")
	}
	out := Baseline{Hits: map[string]int{}}
	for line := 2; scanner.Scan(); line++ {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 3 {
			return Baseline{}, fmt.Errorf("releasecheck: counts baseline line %d has %d fields, want 3", line, len(fields))
		}
		version, err1 := strconv.Atoi(fields[0])
		hits, err2 := strconv.Atoi(fields[2])
		if err1 != nil || err2 != nil {
			return Baseline{}, fmt.Errorf("releasecheck: counts baseline line %d has a non-numeric field", line)
		}
		out.DetectorVersion = version
		out.Hits[fields[1]] = hits
	}
	return out, scanner.Err()
}

// WriteBaseline writes counts_baseline.tsv in construction order.
func WriteBaseline(w io.Writer, b Baseline, order []string) error {
	var s strings.Builder
	s.WriteString(BaselineHeader + "\n")
	for _, id := range order {
		fmt.Fprintf(&s, "%d\t%s\t%d\n", b.DetectorVersion, id, b.Hits[id])
	}
	_, err := io.WriteString(w, s.String())
	return err
}
