package domain

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDeckPreparationFailureBecomesAnOmission(t *testing.T) {
	for _, test := range []struct {
		name         string
		isIdiom      bool
		class, code  string
		wantOmission bool
	}{
		{
			name: "an idiom with a validation failure becomes an omission", isIdiom: true,
			class: "validation", code: "invalid_response", wantOmission: true,
		},
		{
			name: "an idiom with any other code becomes an omission", isIdiom: true,
			class: "validation", code: "unsupported_status", wantOmission: true,
		},
		{
			name: "an idiom with another error class becomes an omission", isIdiom: true,
			class: "transport", code: "timeout", wantOmission: true,
		},
		{
			name: "a lemma with invalid response becomes an omission", class: "validation",
			code: "invalid_response", wantOmission: true,
		},
		{
			name: "a lemma with invalid translation response becomes an omission", class: "validation",
			code: "invalid_translation_response", wantOmission: true,
		},
		{
			name: "a lemma with malformed response becomes an omission", class: "validation",
			code: "malformed_response", wantOmission: true,
		},
		{
			name: "a lemma with response too large becomes an omission", class: "validation",
			code: "response_too_large", wantOmission: true,
		},
		{
			name: "a lemma with any other code fails the deck", class: "validation",
			code: "unsupported_status", wantOmission: false,
		},
		{
			name: "a lemma with another error class fails the deck", class: "transport",
			code: "invalid_response", wantOmission: false,
		},
		{
			name: "an unset kind behaves as a lemma", class: "validation",
			code: "invalid_response", wantOmission: true,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			require.Equal(t, test.wantOmission, IsDeckPreparationFailureOmission(test.isIdiom, test.class, test.code))
		})
	}
}
