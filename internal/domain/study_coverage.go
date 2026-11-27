package domain

// StudyCoverageIndex answers, for a word occurrence's effective vocabulary
// identity, whether the learner has the word in study and whether it is Mature
// (ADR 0136 §3). It is the one place a Book's coverage reads Study progress, so
// the Vocabulary coverage band, Analysis Insights and My Books agree.
//
// Only word items count: an Idiom never changes word-token coverage. A
// withdrawn word has no valid occurrence left and counts as neither.
type StudyCoverageIndex struct {
	mature, inStudy map[StudyItem]bool
}

// NewStudyCoverageIndex indexes the learner's Study progress.
func NewStudyCoverageIndex(progress []StudyProgress) StudyCoverageIndex {
	index := StudyCoverageIndex{mature: map[StudyItem]bool{}, inStudy: map[StudyItem]bool{}}
	for _, p := range progress {
		if p.IsIdiom() || p.Withdrawn {
			continue
		}
		index.inStudy[p.StudyItem] = true
		if IsStudyMature(p) {
			index.mature[p.StudyItem] = true
		}
	}
	return index
}

// Classify reports whether the occurrence's effective identity is in study and
// whether it is Mature. Mature implies in study.
func (i StudyCoverageIndex) Classify(lemma LemmaOccurrence) (inStudy, mature bool) {
	item := WordItem(lemma.Language, lemma.CanonicalLemma, lemma.UPOS, lemma.LexicalDiscriminator)
	return i.inStudy[item], i.mature[item]
}
