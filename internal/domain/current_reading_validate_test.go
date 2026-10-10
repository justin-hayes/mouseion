package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentReadingIdentityValidate(t *testing.T) {
	valid := CurrentReading{OwnerID: "owner", Language: "de", BookID: "book"}
	require.NoError(t, valid.Validate(), "valid primary goal rejected")

	for _, goal := range []CurrentReading{
		{BookID: "book"},
		{OwnerID: "owner", BookID: "book"},
		{OwnerID: "owner", Language: " ", BookID: "book"},
		{OwnerID: " ", Language: "de", BookID: "book"},
		{OwnerID: "owner", Language: "de", BookID: "\t"},
	} {
		assert.Error(t, goal.Validate(), "invalid primary goal accepted: %+v", goal)
	}
}
