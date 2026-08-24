package lemmadisplay

import "testing"

func TestFormat(t *testing.T) {
	tests := []struct {
		name, language, lemma, upos, want string
	}{
		{name: "German noun", language: "de", lemma: "haus", upos: "NOUN", want: "Haus"},
		{name: "German noun locale", language: "de-DE", lemma: "überraschung", upos: "NOUN", want: "Überraschung"},
		{name: "German verb", language: "de", lemma: "gehen", upos: "VERB", want: "gehen"},
		{name: "German proper noun", language: "de", lemma: "goethe", upos: "PROPN", want: "goethe"},
		{name: "German adjective", language: "de", lemma: "schnell", upos: "ADJ", want: "schnell"},
		{name: "non-German noun", language: "en", lemma: "house", upos: "NOUN", want: "house"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Format(tt.language, tt.lemma, tt.upos); got != tt.want {
				t.Fatalf("Format(%q, %q, %q) = %q, want %q", tt.language, tt.lemma, tt.upos, got, tt.want)
			}
		})
	}
}
