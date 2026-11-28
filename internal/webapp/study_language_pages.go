package webapp

import (
	"log"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

// studyLanguagePageKind names a surface whose study language is carried by the
// URL (ADR 0131), such as Reading at /de/reading.
type studyLanguagePageKind string

const (
	pageKindReading       studyLanguagePageKind = "reading"
	pageKindLegacyLibrary studyLanguagePageKind = "legacy-library"
	pageKindReadingSwitch studyLanguagePageKind = "reading-switch"
	pageKindCorpus        studyLanguagePageKind = "corpus"
	pageKindConcordance   studyLanguagePageKind = "concordance"
	pageKindSentenceStudy studyLanguagePageKind = "sentence-study"
	pageKindStudy         studyLanguagePageKind = "study"
	pageKindStudyReports  studyLanguagePageKind = "study-reports"
)

// studyLanguagePage describes one per-language surface. Path is the route
// beneath the language prefix. Landing is the kind a deep page falls back to
// when the language changes (a lemma or sentence does not survive the change);
// a surface's own landing page names itself. Successor is set on a retired
// address (the former Reading page at /library): it redirects to the
// successor page in one hop, keeping the query.
type studyLanguagePage struct {
	Path      string
	Landing   studyLanguagePageKind
	Successor studyLanguagePageKind
}

// studyLanguagePages is the page-kind registry every link builder, the
// resolver and the picker target read. A surface registers its route with
// Handler.perLanguage.
var studyLanguagePages = map[studyLanguagePageKind]studyLanguagePage{
	pageKindReading:       {Path: "reading", Landing: pageKindReading},
	pageKindLegacyLibrary: {Path: "library", Landing: pageKindLegacyLibrary, Successor: pageKindReading},
	pageKindReadingSwitch: {Path: "reading/switch", Landing: pageKindReadingSwitch},
	pageKindCorpus:        {Path: "corpus", Landing: pageKindConcordance},
	pageKindConcordance:   {Path: "corpus/concordance", Landing: pageKindConcordance},
	pageKindSentenceStudy: {Path: "corpus/concordance/sentence", Landing: pageKindConcordance},
	pageKindStudy:         {Path: "study", Landing: pageKindStudy},
	pageKindStudyReports:  {Path: "study/reports", Landing: pageKindStudy},
}

// legacyLanguagePages are retired addresses and the page kind that replaced
// each. The Vocabulary destination became Corpus; its addresses never return,
// so each one redirects straight to its successor in a single hop.
var legacyLanguagePages = map[string]studyLanguagePageKind{
	"vocabulary":                      pageKindCorpus,
	"vocabulary/concordance":          pageKindConcordance,
	"vocabulary/concordance/sentence": pageKindSentenceStudy,
}

// legacyLanguage registers the unprefixed route of a retired address. It
// resolves the study language as an unprefixed page does and redirects to the
// successor with the query intact.
func (h *Handler) legacyLanguage(path string, successor studyLanguagePageKind) {
	h.mux.Handle("GET /"+path, h.user(h.unprefixedLanguagePage(successor, nil)))
}

// legacyPrefixedPage redirects /{language}/<retired path> to the successor. A
// language the learner has nothing in recovers as the successor's own prefixed
// page would.
func (h *Handler) legacyPrefixedPage(successor studyLanguagePageKind) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view := shellViewFromContext(r.Context())
		language := canonicalization.NormalizeLanguage(r.PathValue("lang"))
		rest := "/" + studyLanguagePages[successor].Path
		if view == nil || !view.hasLanguage(language) {
			path, rawQuery := languageFallback(view, successor, rest, r.URL.RawQuery)
			redirectLocal(w, r, path, rawQuery)
			return
		}
		redirectLocal(w, r, studyLanguagePath(successor, language), r.URL.RawQuery)
	})
}

// studyLanguagePath is the address of a page kind: prefixed with the language
// when one is known, otherwise the unprefixed route.
func studyLanguagePath(kind studyLanguagePageKind, language string) string {
	page, ok := studyLanguagePages[kind]
	if !ok {
		return "/"
	}
	language = canonicalization.NormalizeLanguage(language)
	if language == "" {
		return "/" + page.Path
	}
	return "/" + language + "/" + page.Path
}

// studyLanguagePickerTarget is where the shell picker opens kind in language:
// the kind's landing page, with every language-specific detail dropped. An
// unknown kind falls back to Reading.
func studyLanguagePickerTarget(kind studyLanguagePageKind, language string) string {
	landing, ok := studyLanguagePages[kind]
	if !ok {
		return studyLanguagePath(pageKindReading, language)
	}
	return studyLanguagePath(landing.Landing, language)
}

// languagePrefixed is a per-language route registered through perLanguage.
type languagePrefixed struct {
	kind    studyLanguagePageKind
	handler http.Handler
}

// perLanguageRetired registers a retired address that redirects to its
// successor page in one hop, keeping the query. A prefixed request keeps its
// language; an unprefixed one takes the Last study language, as the successor's
// own unprefixed address does.
func (h *Handler) perLanguageRetired(kind studyLanguagePageKind) {
	successor := studyLanguagePages[kind].Successor
	prefixed := func(w http.ResponseWriter, r *http.Request) {
		language := canonicalization.NormalizeLanguage(r.PathValue("lang"))
		redirectLocal(w, r, studyLanguagePath(successor, language), r.URL.RawQuery)
	}
	unprefixed := func(w http.ResponseWriter, r *http.Request) {
		redirectLocal(w, r, studyLanguagePath(successor, ""), r.URL.RawQuery)
	}
	h.perLanguageWith(kind, successor, prefixed, unprefixed)
}

// perLanguage registers a GET surface. It is served under /{language}/<path>,
// and its unprefixed route redirects to the prefixed form.
func (h *Handler) perLanguage(kind studyLanguagePageKind, handler http.HandlerFunc) {
	h.perLanguageWith(kind, kind, handler, handler)
}

// perLanguageWith serves the prefixed route with prefixed and the unprefixed
// route as the page kind target, with unprefixed.
func (h *Handler) perLanguageWith(kind, target studyLanguagePageKind, prefixed, unprefixed http.HandlerFunc) {
	page := studyLanguagePages[kind]
	if h.languagePages == nil {
		h.languagePages = map[string]languagePrefixed{}
	}
	h.languagePages[page.Path] = languagePrefixed{kind: kind, handler: prefixed}
	h.mux.Handle("GET /"+page.Path, h.user(h.unprefixedLanguagePage(target, unprefixed)))
}

// isLanguageSegment reports whether a path segment has the shape of a language
// prefix: a two or three letter code, optionally with a locale or script
// suffix such as de-DE or de_DE. Every other first segment of a route is longer
// or not of this shape, so the shape alone separates a prefix from a route.
func isLanguageSegment(segment string) bool {
	code, suffix, hasSuffix := strings.Cut(strings.ReplaceAll(segment, "_", "-"), "-")
	if len(code) < 2 || len(code) > 3 || !isASCIIAlpha(code) {
		return false
	}
	return !hasSuffix || (len(suffix) >= 2 && len(suffix) <= 8 && isASCIIAlpha(suffix))
}

func isASCIIAlpha(value string) bool {
	for _, r := range value {
		if (r < 'a' || r > 'z') && (r < 'A' || r > 'Z') {
			return false
		}
	}
	return true
}

// matchLanguagePage finds the per-language route for /{language}/<path>.
func (h *Handler) matchLanguagePage(r *http.Request) (http.Handler, bool) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		return nil, false
	}
	prefix, rest, ok := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	if !ok || !isLanguageSegment(prefix) {
		return nil, false
	}
	if successor, retired := legacyLanguagePages[rest]; retired {
		r.SetPathValue("lang", prefix)
		return h.user(h.legacyPrefixedPage(successor)), true
	}
	page, ok := h.languagePages[rest]
	if !ok {
		return nil, false
	}
	r.SetPathValue("lang", prefix)
	return h.user(h.prefixedLanguagePage(page)), true
}

// prefixedLanguagePage resolves the prefix to a page language. A language the
// learner has no Books or vocabulary in redirects to the same page under the
// Last study language; a non-canonical spelling redirects to the canonical one.
func (h *Handler) prefixedLanguagePage(page languagePrefixed) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view := shellViewFromContext(r.Context())
		raw := r.PathValue("lang")
		language := canonicalization.NormalizeLanguage(raw)
		rest := strings.TrimPrefix(r.URL.Path, "/"+raw)
		if view == nil || !view.hasLanguage(language) {
			path, rawQuery := languageFallback(view, page.kind, rest, r.URL.RawQuery)
			redirectLocal(w, r, path, rawQuery)
			return
		}
		if language != raw {
			redirectLocal(w, r, "/"+language+rest, r.URL.RawQuery)
			return
		}
		changed := view.LastLanguage != language
		view.LastLanguage = language
		view.PageLanguage = language
		view.PageKind = page.kind
		if changed {
			// The count loaded with the shell was for the stored language.
			if err := h.loadStudyDue(r.Context(), view, user(r).ID); err != nil {
				fail(w, err)
				return
			}
		}
		if !changed {
			page.handler.ServeHTTP(w, r)
			return
		}
		// The Last study language is recorded only once the page renders: a
		// refused request, such as a stale Browse origin, leaves it alone.
		recorder := &lastLanguageRecorder{ResponseWriter: w, record: func() error {
			return h.services.Store.Shell.SetActiveStudyLanguage(r.Context(), user(r).ID, language)
		}}
		page.handler.ServeHTTP(recorder, r)
	})
}

// lastLanguageRecorder records the page's language as the Last study language
// when the response commits with a success status, before any byte is sent.
type lastLanguageRecorder struct {
	http.ResponseWriter
	record    func() error
	committed bool
}

func (w *lastLanguageRecorder) WriteHeader(status int) {
	if !w.committed {
		w.committed = true
		if status >= http.StatusOK && status < http.StatusMultipleChoices {
			if err := w.record(); err != nil {
				log.Printf("mouseion: record last study language: %v", err)
			}
		}
	}
	w.ResponseWriter.WriteHeader(status)
}

func (w *lastLanguageRecorder) Write(body []byte) (int, error) {
	if !w.committed {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *lastLanguageRecorder) Unwrap() http.ResponseWriter { return w.ResponseWriter }

// languageFallback is where a prefix the learner has nothing in recovers to: the
// same page under the Last study language, or the unprefixed page when the
// learner has none. A deep page (a sentence) does not survive the change of
// language, so it goes to its surface's landing page with its query dropped,
// as the picker does.
func languageFallback(view *shellView, kind studyLanguagePageKind, rest, rawQuery string) (path, query string) {
	if view == nil || view.LastLanguage == "" {
		return rest, rawQuery
	}
	if successor := studyLanguagePages[kind].Successor; successor != "" {
		return studyLanguagePath(successor, view.LastLanguage), rawQuery
	}
	if landing := studyLanguagePages[kind].Landing; landing != kind {
		return studyLanguagePath(landing, view.LastLanguage), ""
	}
	return "/" + view.LastLanguage + rest, rawQuery
}

// unprefixedLanguagePage redirects an unprefixed address to the prefixed form.
// The Last study language is used. A learner with no study language is served
// the unprefixed Reading empty state; every other surface redirects there.
func (h *Handler) unprefixedLanguagePage(kind studyLanguagePageKind, handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view := shellViewFromContext(r.Context())
		language := view.lastLanguage()
		// An old ?language= link keeps its language when the learner has it, and
		// the parameter is stripped either way.
		query := r.URL.Query()
		if queried := canonicalization.NormalizeLanguage(query.Get("language")); view.hasLanguage(queried) {
			language = queried
		}
		query.Del("language")
		if language == "" && kind != pageKindReading {
			// Without a study language every other surface has nothing to show;
			// the unprefixed Reading empty state says how to begin.
			redirectLocal(w, r, "/"+studyLanguagePages[pageKindReading].Path, "")
			return
		}
		if language == "" {
			if view != nil {
				view.PageKind = kind
			}
			handler.ServeHTTP(w, r)
			return
		}
		redirectLocal(w, r, studyLanguagePath(kind, language), query.Encode())
	})
}

// redirectLocal redirects to a local path with an already encoded query. HTMX
// partial requests navigate the whole page, as Reading redirects do.
func redirectLocal(w http.ResponseWriter, r *http.Request, path, rawQuery string) {
	if rawQuery != "" {
		path += "?" + rawQuery
	}
	redirectMyBooksBrowse(w, r, path)
}

// studyLanguageSwitch is the picker's GET target. It resolves the current page
// kind and the chosen language into the prefixed URL.
func (h *Handler) studyLanguageSwitch(w http.ResponseWriter, r *http.Request) {
	view := shellViewFromContext(r.Context())
	language := canonicalization.NormalizeLanguage(strings.TrimSpace(r.URL.Query().Get("lang")))
	if language == "" {
		redirect(w, r, studyLanguagePath(pageKindReading, view.lastLanguage()))
		return
	}
	if view == nil || !view.hasLanguage(language) {
		http.Error(w, "choose a current study language", http.StatusBadRequest)
		return
	}
	redirect(w, r, studyLanguagePickerTarget(studyLanguagePageKind(r.URL.Query().Get("to")), language))
}
