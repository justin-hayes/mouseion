package enrichment

import (
	"context"
	"strings"
	"unicode"
)

type FrequencyLookup interface {
	FrequencyPercentile(context.Context, string, string, string) (float64, bool, error)
}
type DWDSFrequency struct {
	lookup  FrequencyLookup
	version string
}

func NewDWDSFrequency(lookup FrequencyLookup, version string) *DWDSFrequency {
	return &DWDSFrequency{lookup, version}
}
func (p *DWDSFrequency) Name() string    { return "DWDS" }
func (p *DWDSFrequency) Version() string { return p.version }
func (p *DWDSFrequency) Frequency(ctx context.Context, id Identity) (float64, bool, error) {
	return p.lookup.FrequencyPercentile(ctx, id.Language, id.CanonicalLemma, id.UPOS)
}

type StanzaMorphology struct{ version string }

func NewStanzaMorphology(version string) *StanzaMorphology { return &StanzaMorphology{version} }
func (p *StanzaMorphology) Name() string                   { return "Stanza" }
func (p *StanzaMorphology) Version() string                { return p.version }
func (p *StanzaMorphology) Morphology(_ context.Context, c Candidate) (map[string]string, bool, error) {
	if len(c.Morphology) == 0 {
		return nil, false, nil
	}
	out := make(map[string]string, len(c.Morphology))
	for k, v := range c.Morphology {
		out[k] = v
	}
	return out, true, nil
}

type GermanIPA struct{}

func (GermanIPA) Name() string    { return "german-rule-ipa" }
func (GermanIPA) Version() string { return "1" }
func (GermanIPA) Pronunciation(_ context.Context, id Identity) (string, bool, error) {
	if !strings.EqualFold(id.Language, "de") || strings.TrimSpace(id.CanonicalLemma) == "" {
		return "", false, nil
	}
	w := strings.ToLower(strings.TrimSpace(id.CanonicalLemma))
	replacements := []struct{ old, new string }{{"tsch", "tʃ"}, {"sch", "ʃ"}, {"ch", "x"}, {"ei", "aɪ̯"}, {"ie", "iː"}, {"eu", "ɔʏ̯"}, {"äu", "ɔʏ̯"}, {"au", "aʊ̯"}, {"ph", "f"}, {"qu", "kv"}, {"sp", "ʃp"}, {"st", "ʃt"}, {"z", "t͡s"}, {"w", "v"}, {"v", "f"}, {"ß", "s"}, {"ä", "ɛ"}, {"ö", "ø"}, {"ü", "y"}, {"c", "k"}}
	for _, r := range replacements {
		w = strings.ReplaceAll(w, r.old, r.new)
	}
	var b strings.Builder
	for _, r := range w {
		if unicode.IsLetter(r) || strings.ContainsRune("ʃʏɪɔɛøː̯͡", r) {
			b.WriteRune(r)
		}
	}
	if b.Len() == 0 {
		return "", false, nil
	}
	return "/" + b.String() + "/", true, nil
}
