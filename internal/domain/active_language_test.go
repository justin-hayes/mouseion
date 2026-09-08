package domain

import "testing"

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
			if got := ResolveActiveStudyLanguage(languages, test.stored, test.recent); got != test.want {
				t.Fatalf("resolved language=%q, want %q", got, test.want)
			}
		})
	}
	if got := ResolveActiveStudyLanguage([]StudyLanguage{{Language: "de"}}, "it", "it"); got != "de" {
		t.Fatalf("sole language resolution=%q, want de", got)
	}
}
