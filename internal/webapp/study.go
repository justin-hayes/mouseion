package webapp

import (
	"log"
	"net/http"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// studyPageView is what the Study page renders for one study language.
type studyPageView struct {
	Language, LanguageLabel string
	// Available is false for a language whose card policy has no contextual
	// Gloss to anchor a learned meaning (Latin and Ancient Greek).
	Available bool
	Counts    domain.StudyMaterialCounts
}

// studyAvailable reports whether Study serves a study language: only those on
// the modern-language provider route (ADR 0133, ADR 0134 §1).
func studyAvailable(language string) bool {
	return !cardpolicy.For(language).SourceEvidence
}

// study serves /{language}/study. Opening it asks for Study material to be
// prepared ahead of the new words Today will offer; it then reports how many
// are ready and how many are being prepared. It never calls a provider.
func (h *Handler) study(w http.ResponseWriter, r *http.Request) {
	owner := user(r).ID
	language, label := activeStudyLanguageForContext(r.Context())
	view := studyPageView{Language: language, LanguageLabel: label, Available: studyAvailable(language)}
	if view.Available {
		if h.services.Study != nil {
			// Preparation is best effort: the page still reports what is ready.
			if err := h.services.Study.EnqueueStudyPreparation(r.Context(), owner, language); err != nil {
				log.Printf("mouseion: enqueue study preparation: %v", err)
			}
		}
		counts, err := h.services.Store.Study.CountStudyMaterial(r.Context(), owner, language)
		if err != nil {
			fail(w, err)
			return
		}
		view.Counts = counts
	}
	render(w, r, StudyPage(user(r), h.csrf(w, r), view))
}
