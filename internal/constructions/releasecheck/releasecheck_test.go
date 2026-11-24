package releasecheck

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"math"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/constructions"
)

var greek = regexp.MustCompile(`[\x{0370}-\x{03FF}\x{1F00}-\x{1FFF}]`)

func TestWilsonMatchesTheResearchFigures(t *testing.T) {
	p, lo, hi := Wilson(110, 120)
	assert.InDelta(t, 0.917, p, 0.001)
	assert.InDelta(t, 0.853, lo, 0.001)
	assert.InDelta(t, 0.954, hi, 0.001)
	p, _, _ = Wilson(0, 0)
	assert.True(t, math.IsNaN(p))
}

func TestStratifiedReproducesTheConditionalEstimate(t *testing.T) {
	p, lo, hi := Stratified([]Stratum{
		{True: 111, Judged: 120, Weight: 1766},
		{True: 24, Judged: 40, Weight: 73},
	})
	assert.InDelta(t, 0.912, p, 0.001)
	assert.InDelta(t, 0.866, lo, 0.001)
	assert.InDelta(t, 0.958, hi, 0.001)
	nan, _, _ := Stratified([]Stratum{{True: 1, Judged: 0, Weight: 5}})
	assert.True(t, math.IsNaN(nan))
	nan, _, _ = Stratified(nil)
	assert.True(t, math.IsNaN(nan))
}

func TestClearsNeedsPrecisionLowerBoundAndSample(t *testing.T) {
	assert.True(t, Clears(0.91, 0.85, 120, MinJudgedConstruction))
	assert.False(t, Clears(0.89, 0.85, 120, MinJudgedConstruction), "precision below 0.90")
	assert.False(t, Clears(0.95, 0.83, 120, MinJudgedConstruction), "lower bound below 0.84")
	assert.False(t, Clears(0.95, 0.90, 119, MinJudgedConstruction), "too few judged")
	assert.True(t, Clears(0.95, 0.90, 60, MinJudgedSubtype))
}

const researchHeader2607 = "item\twork\tsent_id\tanchor\tconstruction\tsample\tdetector_subtype\tagent_true\tagent_subtype\tjustin_true\tjustin_subtype\n"
const researchHeader2615 = "item\twork\tsent_id\tanchor\tconstruction\tsample\tdetector_subtype\tprior_item_2607\tagent_true\tagent_subtype\tjustin_true\tjustin_subtype\n"

func TestConvertMergesDedupesAndMapsJudgments(t *testing.T) {
	first := researchHeader2607 +
		"I1\tw1\t1\tt_1\tconditional\thit\t\tyes\t\t\t\n" + // superseded by prior_item_2607
		"I2\tw1\t2\tt_2\tresult\thit\tnatural\tyes\tnatural\t\t\n" + // same key as a 2615 row
		"I3\tw1\t3\tt_3\tindirect_statement\thit\thoti-hos\tno\t\tyes\thoti-hos\n" + // justin over agent
		"I4\tw1\t4\tt_4\tconditional\thit\t\tunsure\t\t\t\n" + // dropped
		"I5\tw1\t5\tt_5\tgenitive_absolute\tabstain\tparticiple\tyes\t\t\t\n"
	second := researchHeader2615 +
		"R1\tw1\t1\tt_1\tconditional\thit\t\tI1\tno\t\tyes\t\n" +
		"R2\tw1\t2\tt_2\tresult\thit\tnatural\t\tyes\tnatural\t\t\n" +
		"R3\tw1\t6\tt_6\tarticular_infinitive\thit-topup\tacc\t\tyes\tacc\t\t\n" +
		"R4\tw1\t7\tt_7\tconditional\texcluded\twhether head\t\tno\t\t\t\n"
	judgments, stats, err := Convert(strings.NewReader(first), strings.NewReader(second))
	require.NoError(t, err)

	assert.Equal(t, ConvertStats{Rows2607: 5, Rows2615: 4, Superseded: 2, Unsure: 1, Kept: 6}, stats)
	byItem := map[string]Judgment{}
	for _, j := range judgments {
		byItem[j.Item] = j
	}
	t.Run("a 2615 row replaces its 2607 row", func(t *testing.T) {
		assert.NotContains(t, byItem, "I1")
		assert.NotContains(t, byItem, "I2")
		assert.Equal(t, Measurement2615, byItem["R1"].Measurement)
	})
	t.Run("justin's judgment beats the agent's", func(t *testing.T) {
		assert.True(t, byItem["R1"].Judgment)
		assert.Equal(t, "justin", byItem["R1"].Judge)
		assert.True(t, byItem["I3"].Judgment)
		assert.Equal(t, "hoti_hos", byItem["I3"].JudgedSubtype)
		assert.Equal(t, "agent", byItem["R2"].Judge)
	})
	t.Run("unsure rows are dropped", func(t *testing.T) {
		assert.NotContains(t, byItem, "I4")
	})
	t.Run("subtype names are the detector's", func(t *testing.T) {
		assert.Equal(t, "accusative", byItem["R3"].Subtype)
		assert.Equal(t, "accusative", byItem["R3"].JudgedSubtype)
		assert.Equal(t, "hoti_hos", byItem["I3"].Subtype)
	})
	t.Run("excluded and abstain rows keep their detail", func(t *testing.T) {
		assert.Equal(t, "whether head", byItem["R4"].StratumDetail)
		assert.Empty(t, byItem["R4"].Subtype)
		assert.Equal(t, "participle", byItem["I5"].StratumDetail)
	})
}

func TestConvertRefusesARaggedTable(t *testing.T) {
	_, _, err := Convert(strings.NewReader(researchHeader2607+"I1\tw\n"), strings.NewReader(researchHeader2615))
	require.Error(t, err)
}

func TestJudgmentsRoundTripAndRefuseBadInput(t *testing.T) {
	rows := []Judgment{{
		Work: "w", SentID: "1", Anchor: "t_1", Construction: "result", Measurement: Measurement2615, Item: "R1",
		Stratum: StratumHit, Subtype: "natural", Judgment: true, JudgedSubtype: "natural", Judge: "agent", EmittedBaseline: true,
	}}
	var buf bytes.Buffer
	require.NoError(t, WriteJudgments(&buf, rows))
	back, err := ParseJudgments(&buf)
	require.NoError(t, err)
	assert.Equal(t, rows, back)

	t.Run("a wrong header", func(t *testing.T) {
		_, err := ParseJudgments(strings.NewReader("a\tb\n"))
		require.Error(t, err)
	})
	t.Run("a repeated key", func(t *testing.T) {
		line := "w\t1\tt_1\tresult\t2615\tR1\thit\t\tnatural\ttrue\tnatural\tagent\ttrue\n"
		_, err := ParseJudgments(strings.NewReader(JudgmentsHeader + "\n" + line + line))
		require.Error(t, err)
	})
	t.Run("a wrong field count", func(t *testing.T) {
		_, err := ParseJudgments(strings.NewReader(JudgmentsHeader + "\nw\t1\n"))
		require.Error(t, err)
	})
	t.Run("a bad boolean", func(t *testing.T) {
		_, err := ParseJudgments(strings.NewReader(JudgmentsHeader + "\nw\t1\tt_1\tresult\t2615\tR1\thit\t\tnatural\tmaybe\tnatural\tagent\ttrue\n"))
		require.Error(t, err)
	})
	t.Run("an empty file", func(t *testing.T) {
		_, err := ParseJudgments(strings.NewReader(""))
		require.Error(t, err)
	})
}

func TestCommittedJudgmentFile(t *testing.T) {
	data, err := os.ReadFile("judgments.tsv")
	require.NoError(t, err)
	t.Run("the header is exact", func(t *testing.T) {
		first, _, _ := strings.Cut(string(data), "\n")
		assert.Equal(t, JudgmentsHeader, first)
	})
	t.Run("no field holds Greek text", func(t *testing.T) {
		assert.False(t, greek.Match(data))
	})
	judgments, err := ParseJudgments(bytes.NewReader(data))
	require.NoError(t, err)
	t.Run("keys are unique and every row is judged", func(t *testing.T) {
		assert.NotEmpty(t, judgments)
		for _, j := range judgments {
			assert.Contains(t, []string{Measurement2607, Measurement2615}, j.Measurement)
			assert.Contains(t, []string{"justin", "agent"}, j.Judge)
		}
	})
	t.Run("the hit strata equal the research tables", func(t *testing.T) {
		tally := func(construction, subtype string) (k, n int) {
			for _, j := range judgments {
				if j.Construction != construction || j.Measurement != Measurement2615 {
					continue
				}
				if subtype == "" && j.Stratum == StratumHit {
					n++
					if j.Judgment {
						k++
					}
				}
				if subtype != "" && (j.Stratum == StratumHit || j.Stratum == StratumHitTopup) && j.Subtype == subtype {
					n++
					if j.Judgment && j.JudgedSubtype == subtype {
						k++
					}
				}
			}
			return k, n
		}
		for _, want := range []struct {
			construction, subtype string
			k, n                  int
		}{
			{"conditional", "", 111, 120}, {"articular_infinitive", "", 110, 120}, {"result", "", 116, 120},
			{"purpose", "", 120, 120}, {"genitive_absolute", "", 109, 120}, {"indirect_statement", "", 113, 120},
			{"result", "natural", 76, 76}, {"result", "actual", 54, 60}, {"articular_infinitive", "accusative", 49, 60},
			{"indirect_statement", "infinitive", 81, 85}, {"indirect_statement", "hoti_hos", 55, 60},
		} {
			k, n := tally(want.construction, want.subtype)
			assert.Equal(t, [2]int{want.k, want.n}, [2]int{k, n}, "%s %s", want.construction, want.subtype)
		}
	})
}

func TestCommittedCountFiles(t *testing.T) {
	counts, err := os.ReadFile("research_counts.tsv")
	require.NoError(t, err)
	parsed, err := ParseResearchCounts(bytes.NewReader(counts))
	require.NoError(t, err)
	assert.Equal(t, 1766, parsed[CountKey{"conditional", StratumHit, "", ""}])
	assert.Equal(t, 73, parsed[CountKey{"conditional", StratumExcluded, "whether head", ""}])
	assert.Equal(t, 2, parsed[CountKey{"genitive_absolute", StratumExcluded, "B-readmit", ""}])
	assert.Equal(t, 4, parsed[CountKey{"result", StratumHit, "", "none"}])

	baseline, err := os.ReadFile("counts_baseline.tsv")
	require.NoError(t, err)
	base, err := ParseBaseline(bytes.NewReader(baseline))
	require.NoError(t, err)
	assert.LessOrEqual(t, base.DetectorVersion, constructions.AGDTDetectorVersion, "the baseline is retained from an earlier version until a reviewed re-baseline")
	assert.Len(t, base.Hits, len(constructions.All))
}

func TestParsersRefuseMalformedCountFiles(t *testing.T) {
	for name, body := range map[string]string{
		"a wrong header":     "x\n",
		"a short line":       CountsHeader + "\na\tb\n",
		"a non-number":       CountsHeader + "\na\thit\t\t\tmany\n",
		"a repeated stratum": CountsHeader + "\na\thit\t\t\t1\na\thit\t\t\t2\n",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseResearchCounts(strings.NewReader(body))
			require.Error(t, err)
		})
	}
	for name, body := range map[string]string{
		"a wrong header": "x\n",
		"a short line":   BaselineHeader + "\n1\ta\n",
		"a non-number":   BaselineHeader + "\n1\ta\tmany\n",
	} {
		t.Run("baseline "+name, func(t *testing.T) {
			_, err := ParseBaseline(strings.NewReader(body))
			require.Error(t, err)
		})
	}
}

func TestBaselineRoundTrips(t *testing.T) {
	var buf bytes.Buffer
	in := Baseline{DetectorVersion: 3, Hits: map[string]int{"a": 5, "b": 7}}
	require.NoError(t, WriteBaseline(&buf, in, []string{"a", "b"}))
	out, err := ParseBaseline(&buf)
	require.NoError(t, err)
	assert.Equal(t, in, out)
}

func TestFlipsRuleAgainstTheBaseline(t *testing.T) {
	key := func(anchor string) Key { return Key{"w", "1", anchor, "result"} }
	row := func(anchor string, judged, baseline bool) Judgment {
		return Judgment{Work: "w", SentID: "1", Anchor: anchor, Construction: "result", Judgment: judged, EmittedBaseline: baseline}
	}
	rows := []Judgment{
		row("a", true, true),   // still emitted: fine
		row("b", true, true),   // true, emitted at baseline, now gone: flip
		row("c", false, false), // false, not emitted at baseline, now emitted: flip
		row("d", false, true),  // false, emitted at baseline, now gone: fine
		row("e", true, false),  // true, newly emitted: fine
		row("f", false, false), // false, still not emitted: fine
	}
	emitted := map[Key]string{key("a"): "", key("c"): "", key("e"): ""}
	failures := Flips(rows, emitted)
	require.Len(t, failures, 2)
	assert.Contains(t, failures[0], "no longer emitted")
	assert.Contains(t, failures[0], "w/1/b/result")
	assert.Contains(t, failures[1], "now emitted")
	assert.Contains(t, failures[1], "w/1/c/result")

	t.Run("a many-row flip is abbreviated", func(t *testing.T) {
		var many []Judgment
		for _, anchor := range []string{"a", "b", "c", "d", "e", "f", "g"} {
			many = append(many, row(anchor, true, true))
		}
		out := Flips(many, map[Key]string{})
		require.Len(t, out, 1)
		assert.Contains(t, out[0], "…")
	})
	t.Run("no flips", func(t *testing.T) {
		assert.Empty(t, Flips(rows[:1], emitted))
	})
}

func TestHitDriftRuleIsTenPercent(t *testing.T) {
	baseline := Baseline{Hits: map[string]int{"a": 100, "b": 100, "c": 100}}
	got := HitDrift(baseline, map[string]int{"a": 110, "b": 111, "c": 89}, []string{"a", "b", "c", "d"})
	require.Len(t, got, 3)
	assert.Contains(t, got[0], "b ungated hits moved from 100 to 111")
	assert.Contains(t, got[0], "fresh random sample of 120")
	assert.Contains(t, got[1], "c ungated hits moved from 100 to 89")
	assert.Contains(t, got[2], "d has no hit count")
}

// fakeGate ships what it is told.
type fakeGate struct {
	constructions map[string]bool
	labels        map[[2]string]bool
}

func (g fakeGate) Ships(id string) bool               { return g.constructions[id] }
func (g fakeGate) ShipsLabel(id, subtype string) bool { return g.labels[[2]string{id, subtype}] }

// syntheticInput builds judged rows for two constructions: "one" with a single
// hit stratum, "two" with hit and excluded strata.
func syntheticInput() Input {
	var judgments []Judgment
	emitted := map[Key]string{}
	add := func(construction, stratum, detail, subtype string, count, trueCount int) {
		for i := range count {
			j := Judgment{
				Work: "w", SentID: stratum + detail + subtype, Anchor: "t_" + construction + string(rune('a'+i%26)) + string(rune('a'+i/26)),
				Construction: construction, Measurement: Measurement2615, Stratum: stratum, StratumDetail: detail,
				Subtype: subtype, Judgment: i < trueCount, JudgedSubtype: subtype, EmittedBaseline: true,
			}
			if !j.Judgment {
				j.JudgedSubtype = ""
			}
			judgments = append(judgments, j)
			emitted[j.Key()] = subtype
		}
	}
	add("one", StratumHit, "", "x", 60, 60)
	add("one", StratumHit, "", "y", 60, 50)
	add("one", StratumHitTopup, "", "x", 30, 30)
	add("one", StratumHit, "", "", 4, 3)
	add("two", StratumHit, "", "", 120, 111)
	add("two", StratumExcluded, "whether head", "", 40, 24)
	return Input{
		Judgments: judgments,
		ResearchCount: map[CountKey]int{
			{"one", StratumHit, "", ""}: 500, {"two", StratumHit, "", ""}: 1766,
			{"two", StratumExcluded, "whether head", ""}: 73, {"one", StratumHit, "", "none"}: 7,
		},
		Baseline: Baseline{DetectorVersion: 1, Hits: map[string]int{"one": 100, "two": 100}},
		Detected: Detected{
			Emitted: emitted, Hits: map[string]int{"one": 100, "two": 100},
			Subtypes: map[string]map[string]int{"one": {"x": 40, "y": 50, "": 10}},
		},
		DetectorVersion: 2,
		Gate:            fakeGate{constructions: map[string]bool{"one": true}, labels: map[[2]string]bool{{"one", "x"}: true, {"one", "y"}: true}},
		Constructions:   []string{"one", "two"},
	}
}

func lineOf(report Report, construction, subtype string) Line {
	for _, l := range report.Lines {
		if l.Construction == construction && l.Subtype == subtype {
			return l
		}
	}
	return Line{}
}

func TestEvaluateComputesPerStratumPrecisionAndVerdicts(t *testing.T) {
	report, err := Evaluate(syntheticInput())
	require.NoError(t, err)

	t.Run("a single stratum uses Wilson", func(t *testing.T) {
		one := lineOf(report, "one", "")
		assert.Equal(t, "hit", one.Strata)
		assert.Equal(t, 124, one.Judged, "the unlabelled hit rows are hit rows too")
		assert.InDelta(t, 113.0/124, one.Precision, 0.001)
		assert.True(t, one.Clears)
		assert.Equal(t, "shipped", one.Verdict())
	})
	t.Run("two strata use the stratified estimator and a held-off verdict", func(t *testing.T) {
		two := lineOf(report, "two", "")
		assert.Equal(t, "excluded/whether head + hit", two.Strata)
		assert.Equal(t, 160, two.Judged)
		assert.InDelta(t, 0.912, two.Precision, 0.001)
		assert.InDelta(t, 0.866, two.Lower, 0.001)
		assert.Equal(t, "held off (clears the bar)", two.Verdict())
	})
	t.Run("a subtype pools hit and hit-topup rows on judged subtype", func(t *testing.T) {
		x := lineOf(report, "one", "x")
		assert.Equal(t, 90, x.Judged)
		assert.InDelta(t, 1.0, x.Precision, 0.001)
		assert.Equal(t, "shipped", x.Verdict())
	})
	t.Run("a shipped subtype below the bar fails condition (a)", func(t *testing.T) {
		y := lineOf(report, "one", "y")
		assert.Equal(t, "shipped, BELOW THE BAR", y.Verdict())
		require.Len(t, report.Failures, 1)
		assert.Contains(t, report.Failures[0], "(a) one/y is shipped below the bar")
	})
	t.Run("a subtype line carries the current Go subtype count", func(t *testing.T) {
		assert.Equal(t, 40, lineOf(report, "one", "x").GoHits)
		assert.Equal(t, 50, lineOf(report, "one", "y").GoHits)
	})
	t.Run("the unlabelled row is counted but never gated", func(t *testing.T) {
		none := lineOf(report, "one", "none")
		assert.Equal(t, 4, none.Judged)
		assert.InDelta(t, 0.75, none.Precision, 0.001)
		assert.Equal(t, 10, none.GoHits)
		assert.Equal(t, 7, none.ResearchHits)
		assert.Equal(t, "unlabelled (not gated)", none.Verdict())
		assert.NotContains(t, strings.Join(report.Failures, "\n"), "one/none")
	})
	t.Run("a construction without labelled subtypes has no unlabelled row", func(t *testing.T) {
		assert.Equal(t, Line{}, lineOf(report, "two", "none"))
	})
	t.Run("the notes count the rows and name both versions", func(t *testing.T) {
		notes := strings.Join(report.Notes, "\n")
		assert.Contains(t, notes, "judgment rows: 314")
		assert.Contains(t, notes, "detector baseline version: 1; current version: 2")
	})
}

func TestEvaluateStratumTakesPartOnlyWhenEmitted(t *testing.T) {
	in := syntheticInput()
	for key := range in.Detected.Emitted {
		if key.Construction == "two" && strings.HasPrefix(key.SentID, StratumExcluded) {
			delete(in.Detected.Emitted, key)
		}
	}
	report, err := Evaluate(in)
	require.NoError(t, err)
	two := lineOf(report, "two", "")
	assert.Equal(t, "hit", two.Strata)
	assert.InDelta(t, 111.0/120, two.Precision, 0.001)
	assert.Equal(t, 0, two.NotEmitted, "judged rows of a non-participating stratum are not warnings")
}

func TestEvaluateFailsClosedOnAMissingResearchCount(t *testing.T) {
	in := syntheticInput()
	delete(in.ResearchCount, CountKey{"two", StratumExcluded, "whether head", ""})
	_, err := Evaluate(in)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "missing research count")
}

func TestEvaluateReportsFlipsAndDrift(t *testing.T) {
	in := syntheticInput()
	in.Detected.Hits["one"] = 200
	for _, j := range in.Judgments {
		if j.Construction == "two" && j.Judgment {
			delete(in.Detected.Emitted, j.Key())
			break
		}
	}
	report, err := Evaluate(in)
	require.NoError(t, err)
	joined := strings.Join(report.Failures, "\n")
	assert.Contains(t, joined, "(b)")
	assert.Contains(t, joined, "(c) one ungated hits moved")
}

func TestEvaluateWithNothingEmittedReportsNoEstimate(t *testing.T) {
	in := syntheticInput()
	in.Detected.Emitted = map[Key]string{}
	report, err := Evaluate(in)
	require.NoError(t, err)
	assert.True(t, math.IsNaN(lineOf(report, "one", "").Precision))
	var out bytes.Buffer
	require.NoError(t, report.Print(&out))
	assert.Contains(t, out.String(), "n/a")
	assert.Contains(t, out.String(), "release check: FAIL")
}

func TestPrintShowsTheTableAndPass(t *testing.T) {
	in := syntheticInput()
	in.Gate = fakeGate{}
	report, err := Evaluate(in)
	require.NoError(t, err)
	var out bytes.Buffer
	require.NoError(t, report.Print(&out))
	text := out.String()
	assert.Contains(t, text, "construction")
	assert.Contains(t, text, "held off")
	assert.Contains(t, text, "release check: PASS")
}

func TestWorkOfIsTheFilePrefix(t *testing.T) {
	assert.Equal(t, "tlg0059.tlg030", WorkOf("tlg0059.tlg030.perseus-grc2_oga020_src.conllu"))
	assert.Equal(t, "plain", WorkOf("plain"))
}

func TestVerifyChecksSizeAndDigest(t *testing.T) {
	data := []byte("abc")
	sum := sha256.Sum256(data)
	digest := hex.EncodeToString(sum[:])
	require.NoError(t, Verify(data, 3, digest))
	require.Error(t, Verify(data, 4, digest))
	require.Error(t, Verify(data, 3, strings.Repeat("0", 64)))
}

type fixtureFetcher struct {
	data  []byte
	err   error
	names []string
}

func (f *fixtureFetcher) Fetch(_ context.Context, pin classical.SourcePin) ([]byte, error) {
	f.names = append(f.names, pin.FileName)
	return f.data, f.err
}

func fixtureDefinitions(t *testing.T, data []byte) []classical.Definition {
	t.Helper()
	sum := sha256.Sum256(data)
	pin := classical.SourcePin{
		Layer: "oga-src-v0.2.0-text", FileName: "tlg0032.tlg006.perseus-grc2_oga020_src.conllu",
		Size: int64(len(data)), SHA256: hex.EncodeToString(sum[:]), Format: classical.SourceFormatOGACoNLLU,
	}
	return []classical.Definition{{
		Key: "xenophon-anabasis-1", Language: "grc", WorkTitle: "Anabasis", WorkAuthor: "Xenophon",
		EditionLabel: "Marchant 1904", CitationStart: "1.1.1", CitationEnd: "1.10.19",
		Source: pin, InterpretationVersion: "oga-agdt-syntax/1",
	}}
}

func TestDetectCorpusKeysOccurrencesBySentenceAndTokenId(t *testing.T) {
	data, err := os.ReadFile("../testdata/oga-src-constructions.conllu")
	require.NoError(t, err)
	fetcher := &fixtureFetcher{data: data}
	definitions := fixtureDefinitions(t, data)
	definitions = append(definitions, definitions[0]) // the same file is fetched once

	detected, err := DetectCorpus(context.Background(), definitions[:1], fetcher)
	require.NoError(t, err)
	assert.NotEmpty(t, detected.Emitted)
	for key := range detected.Emitted {
		assert.Equal(t, "tlg0032.tlg006", key.Work)
		assert.Regexp(t, `^t_\d+_\d+$`, key.Anchor)
	}
	assert.Positive(t, detected.Hits["conditional"])

	t.Run("a repeated file is fetched once", func(t *testing.T) {
		fetcher := &fixtureFetcher{data: data}
		_, err := DetectCorpus(context.Background(), definitions, fetcher)
		require.NoError(t, err)
		assert.Len(t, fetcher.names, 1)
	})
	t.Run("a failed fetch is reported", func(t *testing.T) {
		_, err := DetectCorpus(context.Background(), definitions[:1], &fixtureFetcher{err: errors.New("offline")})
		require.Error(t, err)
	})
	t.Run("a file that does not match its pin is refused", func(t *testing.T) {
		_, err := DetectCorpus(context.Background(), definitions[:1], &fixtureFetcher{data: append([]byte("x"), data...)})
		require.Error(t, err)
	})
	t.Run("a file that does not parse is refused", func(t *testing.T) {
		junk := []byte("not conllu\n")
		_, err := DetectCorpus(context.Background(), fixtureDefinitions(t, junk), &fixtureFetcher{data: junk})
		require.Error(t, err)
	})
	t.Run("a unit that cannot be cut is refused", func(t *testing.T) {
		bad := fixtureDefinitions(t, data)
		bad[0].CitationStart = "nonsense"
		_, err := DetectCorpus(context.Background(), bad, &fixtureFetcher{data: data})
		require.Error(t, err)
	})
}
