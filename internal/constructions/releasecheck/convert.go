package releasecheck

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// ConvertStats counts what Convert read, merged and dropped.
type ConvertStats struct {
	Rows2607, Rows2615 int
	// Superseded counts #2607 rows replaced by a #2615 row.
	Superseded int
	// Unsure counts rows dropped for an unsure or empty final judgment.
	Unsure int
	Kept   int
}

// researchSubtypes maps the research's subtype names to the detector's.
var researchSubtypes = map[string]string{
	"acc": "accusative", "dat": "dative", "gen": "genitive", "nom": "nominative", "hoti-hos": "hoti_hos",
}

func subtypeName(research string) string {
	if mapped, ok := researchSubtypes[research]; ok {
		return mapped
	}
	return research
}

// researchRow is one row of a research occurrences.tsv, by column name.
type researchRow map[string]string

func readResearch(r io.Reader, label string) ([]researchRow, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !scanner.Scan() {
		return nil, fmt.Errorf("releasecheck: %s table is empty", label)
	}
	header := strings.Split(scanner.Text(), "\t")
	var rows []researchRow
	for scanner.Scan() {
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != len(header) {
			return nil, fmt.Errorf("releasecheck: %s row %d has %d fields, want %d", label, len(rows)+1, len(fields), len(header))
		}
		row := researchRow{}
		for i, name := range header {
			row[name] = fields[i]
		}
		rows = append(rows, row)
	}
	return rows, scanner.Err()
}

// Convert builds the judgment rows from the two research tables
// (research/grc-construction-detection-measurement at db6ba186):
// #2607's occurrences.tsv and #2615's. A #2615 row replaces the #2607 row it
// re-drew (its prior_item_2607) or that has the same key, because it already
// carries Justin's #2607 judgment where he gave one. The final judgment is
// Justin's where present, otherwise the agent's; unsure and empty judgments
// are dropped. Notes, Smyth, error and citation columns are not carried.
func Convert(table2607, table2615 io.Reader) ([]Judgment, ConvertStats, error) {
	var stats ConvertStats
	first, err := readResearch(table2607, "2607")
	if err != nil {
		return nil, stats, err
	}
	second, err := readResearch(table2615, "2615")
	if err != nil {
		return nil, stats, err
	}
	stats.Rows2607, stats.Rows2615 = len(first), len(second)

	replaced := map[string]bool{}
	keys2615 := map[Key]bool{}
	for _, row := range second {
		if prior := row["prior_item_2607"]; prior != "" {
			replaced[prior] = true
		}
		keys2615[researchKey(row)] = true
	}
	var out []Judgment
	seen := map[Key]bool{}
	add := func(row researchRow, measurement string) {
		j, ok := convertRow(row, measurement)
		if !ok {
			stats.Unsure++
			return
		}
		if seen[j.Key()] {
			return
		}
		seen[j.Key()] = true
		out = append(out, j)
	}
	for _, row := range first {
		if replaced[row["item"]] || keys2615[researchKey(row)] {
			stats.Superseded++
			continue
		}
		add(row, Measurement2607)
	}
	for _, row := range second {
		add(row, Measurement2615)
	}
	stats.Kept = len(out)
	return out, stats, nil
}

func researchKey(row researchRow) Key {
	return Key{row["work"], row["sent_id"], row["anchor"], row["construction"]}
}

func convertRow(row researchRow, measurement string) (Judgment, bool) {
	verdict, subtype, judge := row["agent_true"], row["agent_subtype"], "agent"
	if row["justin_true"] != "" {
		verdict, judge = row["justin_true"], "justin"
		if row["justin_subtype"] != "" {
			subtype = row["justin_subtype"]
		}
	}
	if verdict != "yes" && verdict != "no" {
		return Judgment{}, false
	}
	j := Judgment{
		Work: row["work"], SentID: row["sent_id"], Anchor: row["anchor"], Construction: row["construction"],
		Measurement: measurement, Item: row["item"], Stratum: row["sample"],
		Judgment: verdict == "yes", JudgedSubtype: subtypeName(subtype), Judge: judge,
	}
	if !j.Judgment {
		j.JudgedSubtype = ""
	}
	switch row["sample"] {
	case StratumExcluded, StratumAbstain:
		j.StratumDetail = row["detector_subtype"]
	default:
		j.Subtype = subtypeName(row["detector_subtype"])
	}
	return j, true
}
