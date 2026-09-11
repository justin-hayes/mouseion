package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveActiveStudyLanguage(t *testing.T) {
	languages := []StudyLanguage{{Language: "de"}, {Language: "it"}}
	for _, test := range []struct {
		name, stored, recent, want string
	}{
		{name: "stored language", stored: "it", recent: "de", want: "it"},
		{name: "invalid stored falls back to recent", stored: "fr", recent: "de", want: "de"},
		{name: "invalid values resolve to none", stored: "fr", recent: "nl", want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, ResolveActiveStudyLanguage(languages, test.stored, test.recent))
		})
	}
	assert.Equal(t, "de", ResolveActiveStudyLanguage([]StudyLanguage{{Language: "de"}}, "it", "it"))
}
