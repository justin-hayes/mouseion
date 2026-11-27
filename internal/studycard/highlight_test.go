package studycard

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestHighlightEnglishTargetSupportsSafeUniqueMultiwordMatches(t *testing.T) {
	tests := []struct{ name, translation, target, want string }{
		{"single", "The house is large.", "house", "The <b>house</b> is large."},
		{"multiword", "She visited the old house yesterday.", "old house", "She visited the <b>old house</b> yesterday."},
		{"entire multiword sentence", "The animal was already roasting when we arrived at the barbecue hut.", "The animal was already roasting when we arrived at the barbecue hut", "The animal was already roasting when we arrived at the barbecue hut."},
		{"rejected sentence is escaped", "The <script>hut</script> is bright.", "The <script>hut</script> is bright", "The &lt;script&gt;hut&lt;/script&gt; is bright."},
		{"single-word sentence", "Home.", "Home", "<b>Home</b>."},
		{"missing", "The building is large.", "house", "The building is large."},
		{"ambiguous", "The house is beside another house.", "house", "The house is beside another house."},
		{"boundary", "The houses are large.", "house", "The houses are large."},
		{"markup", "The <script>alert(1)</script> house.", "house", "The &lt;script&gt;alert(1)&lt;/script&gt; <b>house</b>."},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, HighlightEnglishTarget(test.translation, test.target))
		})
	}
}

func TestHighlightEnglishTargetsRendersOnlyValidatedExcerpts(t *testing.T) {
	translation := "And then the motorcycle knocks Piero over."
	assert.Equal(t, "And then the motorcycle <b>knocks</b> Piero <b>over</b>.", HighlightEnglishTargets(translation, []string{"knocks", "over"}, ""))
	assert.Equal(t, "<b>House</b> stands next to another house.", HighlightEnglishTargets("House stands next to another house.", []string{"House"}, ""))
	assert.Equal(t, escapeField("House stands next to another House."), HighlightEnglishTargets("House stands next to another House.", []string{"House"}, ""), "multiple exact occurrences are ambiguous")
	assert.Equal(t, escapeField(translation), HighlightEnglishTargets(translation, []string{"Piero", "knocks"}, ""), "an invalid alignment must be discarded as a whole")
	assert.Equal(t, "And then the motorcycle <b>knocks over</b> Piero.", HighlightEnglishTargets("And then the motorcycle knocks over Piero.", []string{"knocks over"}, ""))
	assert.Equal(t, "<b>Go</b>!", HighlightEnglishTargets("Go!", []string{"Go"}, ""))
	assert.Equal(t, "The <b>house</b> stands.", HighlightEnglishTargets("The house stands.", nil, "house"), "legacy cache phrase remains readable")
}
