package cardpolicy

import "testing"

func TestGreekPolicyUsesSourceEvidence(t *testing.T) {
	for _, tag := range []string{"grc", "GRC", " grc "} {
		policy := For(tag)
		if !policy.SourceEvidence || !policy.ImportedSourceNotice || policy.NLPAnalysed {
			t.Fatalf("For(%q) = %+v, want a source-evidence, imported-analysis policy", tag, policy)
		}
		if policy.SentenceBoundary != BoundaryTerminalMark {
			t.Fatalf("For(%q).SentenceBoundary = %v, want terminal mark", tag, policy.SentenceBoundary)
		}
		if policy.DictionaryLookup != LookupExactlyOne {
			t.Fatalf("For(%q).DictionaryLookup = %v, want exactly-one", tag, policy.DictionaryLookup)
		}
		if !policy.EvidenceAssistedPass {
			t.Fatalf("For(%q) must allow the evidence-assisted pass", tag)
		}
	}
}

func TestModernLanguagesTakeTheProviderRoute(t *testing.T) {
	for _, tag := range []string{"de", "it", "el", "fr", "", "xx"} {
		policy := For(tag)
		if policy.SourceEvidence || policy.ImportedSourceNotice || policy.EvidenceAssistedPass || !policy.NLPAnalysed {
			t.Fatalf("For(%q) = %+v, want the modern-language provider route", tag, policy)
		}
		if policy.SentenceBoundary != BoundaryCapitalisedStart || policy.DictionaryLookup != LookupStored {
			t.Fatalf("For(%q) = %+v, want capitalised-start boundary and stored lookup", tag, policy)
		}
	}
}
