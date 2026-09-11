package testutil

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExternalDatabaseURL(t *testing.T) {
	t.Run("explicit URL", func(t *testing.T) {
		const want = "postgres://example/test"
		t.Setenv("MOUSEION_TEST_DATABASE_URL", want)
		got, explicit := externalDatabaseURL()
		assert.Equal(t, want, got, "externalDatabaseURL() = %q, %t; want %q, true", got, explicit, want)
		assert.True(t, explicit, "externalDatabaseURL() = %q, %t; want %q, true", got, explicit, want)
	})

	t.Run("legacy fallback", func(t *testing.T) {
		t.Setenv("MOUSEION_TEST_DATABASE_URL", "")
		got, explicit := externalDatabaseURL()
		assert.Equal(t, defaultTestDatabaseURL, got, "externalDatabaseURL() = %q, %t; want %q, false", got, explicit, defaultTestDatabaseURL)
		assert.False(t, explicit, "externalDatabaseURL() = %q, %t; want %q, false", got, explicit, defaultTestDatabaseURL)
	})
}
