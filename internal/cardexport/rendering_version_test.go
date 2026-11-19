package cardexport

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecidePreparedDeckFreshness(t *testing.T) {
	cases := []struct {
		name                             string
		runPresentationVersion           int
		runRenderInputVersion            int
		preparationRenderInputVersion    int
		requestedPresentationVersion     int
		wantUpToDate                     bool
		wantPublishedPresentationVersion int
	}{
		{
			name:                             "both versions current",
			runPresentationVersion:           PresentationVersion,
			runRenderInputVersion:            RenderInputVersion,
			preparationRenderInputVersion:    RenderInputVersion,
			requestedPresentationVersion:     PresentationVersion,
			wantUpToDate:                     true,
			wantPublishedPresentationVersion: PresentationVersion,
		},
		{
			name:                             "presentation version behind",
			runPresentationVersion:           PresentationVersion - 1,
			runRenderInputVersion:            RenderInputVersion,
			preparationRenderInputVersion:    RenderInputVersion,
			requestedPresentationVersion:     PresentationVersion,
			wantUpToDate:                     false,
			wantPublishedPresentationVersion: PresentationVersion,
		},
		{
			name:                             "render input version behind on the run but current on the preparation",
			runPresentationVersion:           PresentationVersion,
			runRenderInputVersion:            RenderInputVersion - 1,
			preparationRenderInputVersion:    RenderInputVersion,
			requestedPresentationVersion:     PresentationVersion,
			wantUpToDate:                     true,
			wantPublishedPresentationVersion: PresentationVersion,
		},
		{
			name:                             "render input version behind on both",
			runPresentationVersion:           PresentationVersion,
			runRenderInputVersion:            RenderInputVersion - 1,
			preparationRenderInputVersion:    RenderInputVersion - 1,
			requestedPresentationVersion:     PresentationVersion,
			wantUpToDate:                     false,
			wantPublishedPresentationVersion: PresentationVersion,
		},
		{
			name:                             "run ahead of the requested presentation version",
			runPresentationVersion:           PresentationVersion + 1,
			runRenderInputVersion:            RenderInputVersion,
			preparationRenderInputVersion:    RenderInputVersion,
			requestedPresentationVersion:     PresentationVersion,
			wantUpToDate:                     true,
			wantPublishedPresentationVersion: PresentationVersion + 1,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			upToDate, presentationVersion := DecidePreparedDeckFreshness(tc.runPresentationVersion, tc.runRenderInputVersion, tc.preparationRenderInputVersion, tc.requestedPresentationVersion)
			assert.Equal(t, tc.wantUpToDate, upToDate)
			assert.Equal(t, tc.wantPublishedPresentationVersion, presentationVersion)
		})
	}
}
