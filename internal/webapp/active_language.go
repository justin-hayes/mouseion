package webapp

import (
	"context"
	"errors"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/study"
)

type activeStudyLanguageOption struct {
	domain.StudyLanguage
	HasBooks   bool
	NewArrival bool
}

// shellView is the per-request shell state. LastLanguage is the Last study
// language (ADR 0131): the remembered value that resolves unprefixed URLs and
// navigation links, never the language of the page in view. PageLanguage is the
// language of the page: the URL's prefix on a per-language page, or the Book's
// language on a Book-addressed page. PageKind is set only while a per-language
// page renders.
type shellView struct {
	LastLanguage string
	PageLanguage string
	PageKind     studyLanguagePageKind
	Options      []activeStudyLanguageOption
	// StudyDue is the reviews still due today in the Last study language: the
	// Study tab's due chip. Zero hides the chip.
	StudyDue int
}

// hasLanguage reports whether language is one of the learner's study or known
// vocabulary languages.
func (v *shellView) hasLanguage(language string) bool {
	if v == nil || language == "" {
		return false
	}
	for _, option := range v.Options {
		if option.Language == language {
			return true
		}
	}
	return false
}

// lastLanguage is the Last study language, empty for a nil view.
func (v *shellView) lastLanguage() string {
	if v == nil {
		return ""
	}
	return v.LastLanguage
}

// pageLanguage is the language the page in view is about: the URL's prefix on a
// per-language page, otherwise the Last study language.
func (v *shellView) pageLanguage() string {
	if v.PageLanguage != "" {
		return v.PageLanguage
	}
	return v.LastLanguage
}

// pageLanguageOrEmpty is pageLanguage, empty for a nil view.
func (v *shellView) pageLanguageOrEmpty() string {
	if v == nil {
		return ""
	}
	return v.pageLanguage()
}

type shellViewContextKey struct{}

func activeStudyLanguageLabel(option activeStudyLanguageOption) string {
	label := option.DisplayName + " (" + option.Language + ")"
	switch option.Language {
	case "de":
		label = "Deutsch"
	case "it":
		label = "Italiano"
	case "el":
		label = "Ελληνικά"
	case "grc", "la":
		// The catalog records the curated display name; never append a code.
		label = option.DisplayName
	}
	if !option.HasBooks {
		label += " (no books)"
	}
	if option.NewArrival {
		label += " (new)"
	}
	return label
}

func shellViewFromContext(ctx context.Context) *shellView {
	view, ok := ctx.Value(shellViewContextKey{}).(*shellView)
	if !ok {
		return nil
	}
	return view
}

func activeStudyLanguageForContext(ctx context.Context) (language, label string) {
	view := shellViewFromContext(ctx)
	if view == nil {
		return "", ""
	}
	language = view.pageLanguage()
	for _, option := range view.Options {
		if option.Language == language {
			return language, option.DisplayName
		}
	}
	return language, language
}

// showStudyLanguagePicker reports whether the shell shows the picker: only on a
// per-language page, whose URL names a language. Job status, the Catalog, and
// Book-addressed pages (whose language is the Book's) have no picker.
func showStudyLanguagePicker(view *shellView, context NavigationContext) bool {
	return view != nil && len(view.Options) > 0 && context != NavigationCatalogs && view.PageKind != ""
}

// shellNavigationPath is a primary navigation link: the page kind under the
// Last study language.
func shellNavigationPath(ctx context.Context, kind studyLanguagePageKind) string {
	return studyLanguagePath(kind, shellViewFromContext(ctx).lastLanguage())
}

func (h *Handler) loadShellView(ctx context.Context, owner string) (*shellView, error) {
	studyLanguages, err := h.services.Store.Shell.ListStudyLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	progressLanguages, err := h.services.Store.Shell.ListStudyProgressLanguages(ctx, owner)
	if err != nil {
		return nil, err
	}
	stored, err := h.services.Store.Shell.GetStoredActiveStudyLanguage(ctx, owner)
	if err != nil {
		return nil, err
	}
	recent, err := h.services.Store.Shell.MostRecentlyActivatedStudyLanguage(ctx, owner)
	if err != nil && !errors.Is(err, domain.ErrNotFound) {
		return nil, err
	}
	recent = canonicalization.NormalizeLanguage(recent)
	stored = canonicalization.NormalizeLanguage(stored)
	active := domain.ResolveActiveStudyLanguage(studyLanguages, stored, recent)
	// A language with Study progress stays selectable after its last Book is removed.
	if !studyLanguagePresent(studyLanguages, stored) && studyLanguagePresent(progressLanguages, stored) {
		active = stored
	}
	view := &shellView{
		LastLanguage: active,
	}
	for _, language := range studyLanguages {
		view.Options = append(view.Options, activeStudyLanguageOption{
			StudyLanguage: language,
			HasBooks:      true,
			NewArrival:    language.Language == recent && language.Language != view.LastLanguage,
		})
	}
	for _, language := range progressLanguages {
		if studyLanguagePresent(studyLanguages, language.Language) {
			continue
		}
		view.Options = append(view.Options, activeStudyLanguageOption{StudyLanguage: language})
	}
	return view, nil
}

// loadStudyDue sets the view's due count for its Last study language. The chip
// is absent, the count zero, when there is no such language, when Study does
// not serve it, or when the handler has no Study store.
func (h *Handler) loadStudyDue(ctx context.Context, view *shellView, owner string) error {
	view.StudyDue = 0
	if view.LastLanguage == "" || !study.Available(view.LastLanguage) || h.services.Store.Study == nil {
		return nil
	}
	due, err := h.services.Store.Study.CountStudyReviewsDue(ctx, owner, view.LastLanguage, h.now())
	if err != nil {
		return err
	}
	view.StudyDue = due
	return nil
}
