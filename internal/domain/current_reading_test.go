package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCurrentReadingValidate(t *testing.T) {
	valid := CurrentReading{OwnerID: "owner", Language: "de", BookID: "book"}
	require.NoError(t, valid.Validate())

	for _, reading := range []CurrentReading{
		{BookID: "book"},
		{OwnerID: "owner", BookID: "book"},
		{OwnerID: "owner", Language: " ", BookID: "book"},
		{OwnerID: " ", Language: "de", BookID: "book"},
		{OwnerID: "owner", Language: "de", BookID: "\t"},
	} {
		assert.Error(t, reading.Validate(), "invalid current reading accepted: %+v", reading)
	}
}
