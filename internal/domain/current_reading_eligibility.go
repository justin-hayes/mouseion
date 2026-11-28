package domain

// CurrentReadingEligibilityIn combines the Analysis evidence classification of
// a Book with the study language the reading would start in. bookLanguage is
// the Book's chosen language, empty when none is chosen.
func CurrentReadingEligibilityIn(classification BookEvidenceClassification, bookLanguage, language string) CurrentReadingEligibilityReason {
	if classification.Eligibility != CurrentReadingEligible {
		return classification.Eligibility
	}
	if bookLanguage != language {
		return CurrentReadingOtherLanguage
	}
	return CurrentReadingEligible
}
