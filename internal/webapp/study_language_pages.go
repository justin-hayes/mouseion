package webapp

import (
	"log"
	"net/http"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

// studyLanguagePageKind names a surface whose study language is carried by the
// URL (ADR 0131), such as My Books at /de/library.
type studyLanguagePageKind string

const (
	pageKindMyBooks       studyLanguagePageKind = "my-books"
	pageKindReading       studyLanguagePageKind = "reading"
	pageKindReadingSwitch studyLanguagePageKind = "reading-switch"
	pageKindVocabulary    studyLanguagePageKind = "vocabulary"
	pageKindConcordance   studyLanguagePageKind = "concordance"
	pageKindSentenceStudy studyLanguagePageKind = "sentence-study"
)

// studyLanguagePage describes one per-language surface. Path is the route
// beneath the language prefix. Prefixed is false until the surface moves under
// the prefix; an unmoved kind keeps its unprefixed route and the picker lands
// on Landing's page instead. Landing is the kind a deep page falls back to when
// the picker changes language (a lemma or sentence does not survive the
// change); a surface's own landing page names itself.
type studyLanguagePage struct {
	Path     string
	Prefixed bool
	Landing  studyLanguagePageKind
}

// studyLanguagePages is the page-kind registry every link builder, the
// resolver and the picker target read. A ticket that moves a surface flips
// Prefixed and registers its route with Handler.perLanguage.
var studyLanguagePages = map[studyLanguagePageKind]studyLanguagePage{
	pageKindMyBooks:       {Path: "library", Prefixed: true, Landing: pageKindMyBooks},
	pageKindReading:       {Path: "reading", Prefixed: true, Landing: pageKindReading},
	pageKindReadingSwitch: {Path: "reading/switch", Prefixed: true, Landing: pageKindReadingSwitch},
	pageKindVocabulary:    {Path: "vocabulary", Prefixed: true, Landing: pageKindConcordance},
	pageKindConcordance:   {Path: "vocabulary/concordance", Prefixed: true, Landing: pageKindConcordance},
	pageKindSentenceStudy: {Path: "vocabulary/concordance/sentence", Prefixed: true, Landing: pageKindConcordance},
}

// studyLanguagePath is the address of a page kind: prefixed with the language
// when the kind has moved and a language is known, otherwise today's
// unprefixed route (which redirects once the kind is prefixed).
func studyLanguagePath(kind studyLanguagePageKind, language string) string {
	page, ok := studyLanguagePages[kind]
	if !ok {
		return "/"
	}
	language = canonicalization.NormalizeLanguage(language)
	if !page.Prefixed || language == "" {
		return "/" + page.Path
	}
	return "/" + language + "/" + page.Path
}

// studyLanguagePickerTarget is where the shell picker opens kind in language:
// the kind's landing page, with every language-specific detail dropped. An
// unmoved kind falls back to My Books, today's landing for a language change.
func studyLanguagePickerTarget(kind studyLanguagePageKind, language string) string {
	landing := studyLanguagePages[kind].Landing
	if page, ok := studyLanguagePages[landing]; !ok || !page.Prefixed {
		landing = pageKindMyBooks
	}
	return studyLanguagePath(landing, language)
}

// languagePrefixed is a per-language route registered through perLanguage.
type languagePrefixed struct {
	kind    studyLanguagePageKind
	handler http.Handler
}

// perLanguage registers a GET surface. A moved kind is served under
// /{language}/<path>, and its unprefixed route redirects to the prefixed form;
// an unmoved kind keeps the plain route.
func (h *Handler) perLanguage(kind studyLanguagePageKind, handler http.HandlerFunc) {
	page := studyLanguagePages[kind]
	if !page.Prefixed {
		h.mux.Handle("GET /"+page.Path, h.user(handler))
		return
	}
	if h.languagePages == nil {
		h.languagePages = map[string]languagePrefixed{}
	}
	h.languagePages[page.Path] = languagePrefixed{kind: kind, handler: handler}
	h.mux.Handle("GET /"+page.Path, h.user(h.unprefixedLanguagePage(kind, handler)))
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
			redirectLocal(w, r, languageFallbackPath(view, rest), r.URL.RawQuery)
			return
		}
		if language != raw {
			redirectLocal(w, r, "/"+language+rest, r.URL.RawQuery)
			return
		}
		view.ActiveLanguage = language
		view.PageLanguage = language
		view.PageKind = page.kind
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

// languageFallbackPath is the same page under the Last study language, or the
// unprefixed page when the learner has none.
func languageFallbackPath(view *shellView, rest string) string {
	if view == nil || view.ActiveLanguage == "" {
		return rest
	}
	return "/" + view.ActiveLanguage + rest
}

// unprefixedLanguagePage redirects an unprefixed address to the prefixed form.
// The Last study language is used. A learner with no study language is served
// the unprefixed My Books empty state; every other surface redirects there.
func (h *Handler) unprefixedLanguagePage(kind studyLanguagePageKind, handler http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		view := shellViewFromContext(r.Context())
		language := view.lastLanguage()
		// A legacy ?language= is dropped, never honoured.
		query := r.URL.Query()
		query.Del("language")
		if language == "" && kind != pageKindMyBooks {
			// Without a study language every other surface has nothing to show;
			// the unprefixed My Books empty state says how to begin.
			redirectLocal(w, r, "/"+studyLanguagePages[pageKindMyBooks].Path, "")
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
// partial requests navigate the whole page, as My Books redirects do.
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
		redirect(w, r, studyLanguagePath(pageKindMyBooks, view.lastLanguage()))
		return
	}
	if view == nil || !view.hasLanguage(language) {
		http.Error(w, "choose a current study language", http.StatusBadRequest)
		return
	}
	redirect(w, r, studyLanguagePickerTarget(studyLanguagePageKind(r.URL.Query().Get("to")), language))
}
