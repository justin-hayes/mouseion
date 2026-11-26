package webapp

import (
	"context"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
	"github.com/justin-hayes/mouseion/internal/study"
)

// sourceCardView is what a source-evidence word's card back shows below the
// usual rows (ADR 0138 §3): the dictionary outline of each cited entry and the
// passage the sentence sits in. It is zero for a modern-language card.
type sourceCardView struct {
	// Entries are the cited entries still in the dictionary, in source order.
	Entries []dictionaryEntryView
	// Passage is the stored passage, or zero when none applies or its
	// translation cannot be credited.
	Passage passageView
	notices []string
	// dictionaryPin and passagePin are the pins the card was shown against.
	dictionaryPin, passagePin string
}

// Notices are the source notices the Study page shows for the card.
func (s sourceCardView) Notices() []string { return s.notices }

// dictionaryEntryView is one cited dictionary entry's compact outline.
type dictionaryEntryView struct {
	// Source names the dictionary; Headword and Number label the entry. Number
	// is the entry number when the policy stacks entries, otherwise zero.
	Source, Headword string
	Number           int
	Outline          []outlineSenseView
	Targets          []targetOutlineView
}

type outlineSenseView struct {
	Number      string
	Level       int
	Definitions string
	// Cited marks a sense the Gloss rests on, shown only while the stored pin is
	// the dictionary's own.
	Cited bool
}

// targetOutlineView is the outline a senseless Lewis & Short entry's resolved
// cross-reference names.
type targetOutlineView struct {
	Reference string
	Outline   []outlineSenseView
}

// passageView is the stored passage with its label.
type passageView struct {
	Label, Text string
}

// Empty reports whether there is no passage to show.
func (p passageView) Empty() bool { return strings.TrimSpace(p.Text) == "" }

// attachSourceEvidence gives a source-evidence word's card its dictionary
// outline and passage. The outline is read from the dictionary at render time;
// the passage is the stored one. A modern-language card, or a card with nothing
// to show, is left as it is.
func attachSourceEvidence(ctx context.Context, lexical enrichment.LexicalProvider, language string, card domain.StudyCard, view *studyCardView) {
	policy := cardpolicy.For(language)
	if !policy.SourceEvidence {
		return
	}
	cited := card.Progress.LearnedCitations
	source := sourceCardView{}
	shownPin := ""
	if resolution, ok := resolveCited(ctx, lexical, policy, card.Progress.CanonicalLemma, cited); ok {
		source.Entries = dictionaryEntryViews(policy, resolution)
		shownPin = resolution.Pin
	}
	passagePin := ""
	if passage := card.Material.Passage; !passage.Empty() {
		if label, err := cardexport.PassageContextLabel(passage.TranslationPin, passage.Citation); err == nil {
			source.Passage = passageView{Label: label, Text: passage.Text}
			passagePin = passage.TranslationPin
		}
	}
	if shownPin == "" {
		// The cited entry may be gone, yet the dictionary in use is still credited.
		shownPin = study.DictionaryPin(lexical, policy)
	}
	source.dictionaryPin, source.passagePin = shownPin, passagePin
	source.notices = sourceNotices(policy, shownPin, passagePin)
	view.Source = source
}

// resolveCited resolves the lemma's entries and keeps those the Gloss cited. The
// cited senses are flagged only when the stored pin is the dictionary's own pin;
// an entry the dictionary no longer holds is left out.
func resolveCited(ctx context.Context, lexical enrichment.LexicalProvider, policy cardpolicy.Policy, lemma string, cited domain.StudyCitations) (sourceevidence.Resolution, bool) {
	flagged := map[sourceevidence.SenseRef]struct{}{}
	for i, id := range cited.SenseIDs {
		flagged[sourceevidence.SenseRef{EntryID: cited.EntryIDs[i], SenseID: id}] = struct{}{}
	}
	resolution, ok := sourceevidence.Resolve(ctx, lexical, policy, lemma, flagged)
	if !ok {
		return sourceevidence.Resolution{}, false
	}
	if strings.TrimSpace(cited.DictionaryPin) != strings.TrimSpace(resolution.Pin) {
		resolution, ok = sourceevidence.Resolve(ctx, lexical, policy, lemma, nil)
		if !ok {
			return sourceevidence.Resolution{}, false
		}
	}
	kept := resolution.Entries[:0:0]
	for _, entry := range resolution.Entries {
		if slices.Contains(cited.EntrySet(), entry.ID) {
			kept = append(kept, entry)
		}
	}
	resolution.Entries = kept
	return resolution, len(kept) > 0
}

func dictionaryEntryViews(policy cardpolicy.Policy, resolution sourceevidence.Resolution) []dictionaryEntryView {
	views := make([]dictionaryEntryView, 0, len(resolution.Entries))
	for _, entry := range resolution.Entries {
		view := dictionaryEntryView{Outline: outlineSenseViews(entry.Outline)}
		if policy.Entries == cardpolicy.EntryLewisShort {
			view.Source, view.Number = "Lewis & Short", entry.Number
			view.Headword = strings.TrimSpace(entry.LewisShort.Orth)
			if view.Headword == "" {
				view.Headword = strings.TrimSpace(entry.LewisShort.Headword)
			}
			for _, target := range entry.Targets {
				view.Targets = append(view.Targets, targetOutlineView{Reference: target.Reference, Outline: outlineSenseViews(target.Outline)})
			}
		} else {
			view.Source, view.Headword = "LSJ", strings.TrimSpace(entry.LSJ.Headword)
		}
		views = append(views, view)
	}
	return views
}

func outlineSenseViews(outline []sourceevidence.OutlineSense) []outlineSenseView {
	views := make([]outlineSenseView, 0, len(outline))
	for _, sense := range outline {
		var definitions []string
		for _, definition := range sense.Definitions {
			if definition = strings.TrimSpace(definition); definition != "" {
				definitions = append(definitions, definition)
			}
		}
		views = append(views, outlineSenseView{Number: strings.TrimSpace(sense.Number), Level: sense.Level, Definitions: strings.Join(definitions, "; "), Cited: sense.Cited})
	}
	return views
}

// sourceNotices are the notices the Study page shows for a source-evidence
// language: the treebank and text source, the dictionary with its pin, and the
// translation of each passage pin, as a deck carries them.
func sourceNotices(policy cardpolicy.Policy, dictionaryPin string, passagePins ...string) []string {
	if !policy.SourceEvidence {
		return nil
	}
	var notices []string
	if policy.SourceNotice != "" {
		notices = append(notices, policy.SourceNotice)
	}
	switch policy.Entries {
	case cardpolicy.EntryLSJ:
		notices = append(notices, dictionary.AncientGreekLSJNotice(dictionaryPin))
	case cardpolicy.EntryLewisShort:
		notices = append(notices, dictionary.LatinLSNotice(dictionaryPin))
	case cardpolicy.EntryNone:
	}
	seen := map[string]struct{}{}
	for _, pin := range passagePins {
		if _, done := seen[pin]; pin == "" || done {
			continue
		}
		seen[pin] = struct{}{}
		if notice, err := cardexport.PassageNotice(pin); err == nil {
			notices = append(notices, notice)
		}
	}
	return notices
}
