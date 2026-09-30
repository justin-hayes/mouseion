package lemmarisk

import (
	"context"
	"errors"
	"testing"
)

type fixtureIndex map[string][]Alternative

func (i fixtureIndex) LemmaExists(_ context.Context, _, lemma, _ string) (bool, error) {
	return lemma == "drache" || lemma == "drachen", nil
}

func (i fixtureIndex) Alternatives(_ context.Context, language, surface, upos, sentence string) ([]Alternative, error) {
	return i[surface+"|"+sentence], nil
}

func TestGermanFlagsRequirePlausibleAlternativeAndIncludeRecurrenceMerges(t *testing.T) {
	const dragon = "Ein Reiter zielt mit seinem Speer auf einen Drachen."
	const kite = "Der Drachen steigt bei starkem Wind hoch in den Himmel."
	occurrences := []Occurrence{
		{ID: "dragon", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: dragon},
		{ID: "other-1", Language: "de", Surface: "Drache", Lemma: "Drache", UPOS: "NOUN", Sentence: "Der Drache schläft."},
		{ID: "other-2", Language: "de", Surface: "Drache", Lemma: "Drache", UPOS: "NOUN", Sentence: "Ein Drache wacht."},
		{ID: "kite", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: kite},
		{ID: "compound", Language: "de", Surface: "Basketballspiel", Lemma: "Basketballspiel", UPOS: "NOUN", Sentence: "Das Basketballspiel beginnt."},
	}
	index := fixtureIndex{
		"Drachen|" + dragon: {{Lemma: "Drache", UPOS: "NOUN", Source: "kaikki", Version: "fixture-1", EvidenceID: "dragon-entry", ContextRelevant: true}},
		"Drachen|" + kite:   {{Lemma: "Drache", UPOS: "NOUN", Source: "kaikki", Version: "fixture-1", EvidenceID: "dragon-entry", ContextRelevant: false}},
	}

	flags, assessed, err := Detect(context.Background(), "de", occurrences, index)
	if err != nil {
		t.Fatal(err)
	}
	if !assessed {
		t.Fatal("German evidence should be assessed when the index responds")
	}
	if len(flags) != 1 || flags[0].OccurrenceID != "dragon" || flags[0].Alternative.Lemma != "Drache" {
		t.Fatalf("flags = %#v, want only the context-supported Drach → Drache occurrence", flags)
	}
}

func TestGermanDoesNotFlagDictionaryMissWithoutAlternative(t *testing.T) {
	occurrences := []Occurrence{{ID: "compound", Language: "de", Surface: "Basketballspiel", Lemma: "Basketballspiel", UPOS: "NOUN", Sentence: "Das Basketballspiel beginnt."}}
	flags, assessed, err := Detect(context.Background(), "de", occurrences, fixtureIndex{})
	if err != nil {
		t.Fatal(err)
	}
	if !assessed || len(flags) != 0 {
		t.Fatalf("assessed=%v flags=%#v, want assessed and no blocking flag", assessed, flags)
	}
}

func TestAutomaticDetectionStaysDisabledOutsideGerman(t *testing.T) {
	occurrences := []Occurrence{{ID: "it", Language: "it", Surface: "case", Lemma: "casa", UPOS: "NOUN", Sentence: "Le case sono grandi."}}
	flags, assessed, err := Detect(context.Background(), "it", occurrences, fixtureIndex{})
	if err != nil {
		t.Fatal(err)
	}
	if assessed || len(flags) != 0 {
		t.Fatalf("assessed=%v flags=%#v, want automatic detection disabled", assessed, flags)
	}
}

func TestNearThresholdAlternativeMustReachRecurringFloor(t *testing.T) {
	occurrences := []Occurrence{
		{ID: "target", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: "Ein Drache lauert."},
		{ID: "one", Language: "de", Surface: "Drache", Lemma: "Drache", UPOS: "NOUN", Sentence: "Der Drache wartet."},
	}
	index := fixtureIndex{"Drachen|Ein Drache lauert.": {{Lemma: "Drache", UPOS: "NOUN", Source: "kaikki", Version: "v1", EvidenceID: "e1", ContextRelevant: true}}}
	flags, assessed, err := Detect(context.Background(), "de", occurrences, index)
	if err != nil {
		t.Fatal(err)
	}
	if !assessed || len(flags) != 0 {
		t.Fatalf("assessed=%v flags=%#v, want assessed with no flag below recurrence floor", assessed, flags)
	}
}

func TestPotentialOccurrencesCanCollectivelyCrossRecurringFloor(t *testing.T) {
	const firstSentence = "Ein Reiter trifft den Drachen mit seinem Speer."
	const secondSentence = "Der Drache erscheint erneut am Horizont."
	occurrences := []Occurrence{
		{ID: "first", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: firstSentence},
		{ID: "second", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: secondSentence},
		{ID: "existing", Language: "de", Surface: "Drache", Lemma: "Drache", UPOS: "NOUN", Sentence: "Ein Drache schläft."},
	}
	index := fixtureIndex{
		"Drachen|" + firstSentence:  {{Lemma: "Drache", UPOS: "NOUN", Source: "kaikki", Version: "v1", EvidenceID: "e1", ContextRelevant: true}},
		"Drachen|" + secondSentence: {{Lemma: "Drache", UPOS: "NOUN", Source: "kaikki", Version: "v1", EvidenceID: "e1", ContextRelevant: true}},
	}
	flags, assessed, err := Detect(context.Background(), "de", occurrences, index)
	if err != nil {
		t.Fatal(err)
	}
	if !assessed || len(flags) != 2 || flags[0].OccurrenceID != "first" || flags[1].OccurrenceID != "second" {
		t.Fatalf("assessed=%v flags=%#v, want both plausible occurrences flagged as a combined floor crossing", assessed, flags)
	}
}

type unavailableIndex struct{}

func (unavailableIndex) LemmaExists(context.Context, string, string, string) (bool, error) {
	return false, errors.New("local index unavailable")
}
func (unavailableIndex) Alternatives(context.Context, string, string, string, string) ([]Alternative, error) {
	return nil, errors.New("local index unavailable")
}

func TestUnavailableIndexIsNotAnAssessment(t *testing.T) {
	occurrences := []Occurrence{{ID: "one", Language: "de", Surface: "Drachen", Lemma: "Drach", UPOS: "NOUN", Sentence: "Ein Drache."}}
	flags, assessed, err := Detect(context.Background(), "de", occurrences, unavailableIndex{})
	if err == nil || assessed || len(flags) != 0 {
		t.Fatalf("flags=%#v assessed=%v err=%v; unavailable reference evidence must not be treated as clean", flags, assessed, err)
	}
}
