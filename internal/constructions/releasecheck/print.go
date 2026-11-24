package releasecheck

import (
	"fmt"
	"io"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

func number(v float64) string {
	if math.IsNaN(v) {
		return "n/a"
	}
	return fmt.Sprintf("%.3f", v)
}

func hitCount(known bool, n int) string {
	if !known {
		return "-"
	}
	return strconv.Itoa(n)
}

func orDash(s string) string {
	if s == "" {
		return "-"
	}
	return s
}

// table pads rows into aligned columns.
func table(rows [][]string) string {
	widths := make([]int, len(rows[0]))
	for _, row := range rows {
		for i, cell := range row {
			widths[i] = max(widths[i], utf8.RuneCountInString(cell))
		}
	}
	var b strings.Builder
	for _, row := range rows {
		for i, cell := range row {
			b.WriteString(cell)
			if i < len(row)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-utf8.RuneCountInString(cell)+2))
			}
		}
		b.WriteByte('\n')
	}
	return b.String()
}

// Print writes the precision table, the notes and the failures.
func (r Report) Print(w io.Writer) error {
	rows := [][]string{{"construction", "subtype", "strata", "judged", "precision", "95% interval", "emitted-only", "not emitted", "go hits", "research hits", "verdict"}}
	for _, l := range r.Lines {
		rows = append(rows, []string{
			l.Construction, orDash(l.Subtype), orDash(l.Strata), strconv.Itoa(l.Judged), number(l.Precision),
			number(l.Lower) + "–" + number(l.Upper), number(l.EmittedOnly), strconv.Itoa(l.NotEmitted),
			hitCount(l.Subtype == "", l.GoHits), hitCount(true, l.ResearchHits), l.Verdict(),
		})
	}
	var b strings.Builder
	b.WriteString(table(rows))
	b.WriteByte('\n')
	for _, note := range r.Notes {
		b.WriteString(note + "\n")
	}
	b.WriteByte('\n')
	if len(r.Failures) == 0 {
		b.WriteString("release check: PASS\n")
	} else {
		b.WriteString("release check: FAIL\n" + strings.Join(r.Failures, "\n") + "\n")
	}
	_, err := io.WriteString(w, b.String())
	return err
}
