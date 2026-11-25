package cardpolicy

import "testing"

func TestGreekPolicyUsesSourceEvidence(t *testing.T) {
	for _, tag := range []string{"grc", "GRC", " grc "} {
		policy := For(tag)
		if !policy.SourceEvidence || policy.SourceNotice != GreekSourceNotice || policy.NLPAnalysed {
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

func TestGreekPolicyNamesItsTranslationsAndEntries(t *testing.T) {
	policy := For("grc")
	if policy.Translations != TranslationPerseus || policy.Entries != EntryLSJ {
		t.Fatalf("For(grc) = %+v, want Perseus translations and LSJ entries", policy)
	}
}

func TestLatinPolicyUsesSourceEvidenceWithNoProviderPass(t *testing.T) {
	for _, tag := range []string{"la", "LA", " la "} {
		policy := For(tag)
		if !policy.SourceEvidence || policy.NLPAnalysed || policy.EvidenceAssistedPass {
			t.Fatalf("For(%q) = %+v, want source-evidence cards, no NLP notice, no evidence-assisted pass", tag, policy)
		}
		if policy.SourceNotice != LatinSourceNotice || policy.Translations != TranslationPerseus {
			t.Fatalf("For(%q) = %+v, want the Latin notice and Perseus translations", tag, policy)
		}
		if policy.SentenceBoundary != BoundaryTerminalMark {
			t.Fatalf("For(%q).SentenceBoundary = %v, want terminal mark", tag, policy.SentenceBoundary)
		}
		if policy.Entries != EntryLewisShort {
			t.Fatalf("For(%q).Entries = %v, want Lewis & Short, never the Greek LSJ", tag, policy.Entries)
		}
	}
}

func TestModernLanguagesTakeTheProviderRoute(t *testing.T) {
	for _, tag := range []string{"de", "it", "el", "fr", "", "xx"} {
		policy := For(tag)
		if policy.SourceEvidence || policy.SourceNotice != "" || policy.EvidenceAssistedPass || !policy.NLPAnalysed {
			t.Fatalf("For(%q) = %+v, want the modern-language provider route", tag, policy)
		}
		if policy.SentenceBoundary != BoundaryCapitalisedStart || policy.DictionaryLookup != LookupStored {
			t.Fatalf("For(%q) = %+v, want capitalised-start boundary and stored lookup", tag, policy)
		}
	}
}
