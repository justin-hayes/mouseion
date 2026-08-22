package testutil

import "testing"

func TestExternalDatabaseURL(t *testing.T) {
	t.Run("explicit URL", func(t *testing.T) {
		const want = "postgres://example/test"
		t.Setenv("MOUSEION_TEST_DATABASE_URL", want)
		got, explicit := externalDatabaseURL()
		if got != want || !explicit {
			t.Fatalf("externalDatabaseURL() = %q, %t; want %q, true", got, explicit, want)
		}
	})

	t.Run("legacy fallback", func(t *testing.T) {
		t.Setenv("MOUSEION_TEST_DATABASE_URL", "")
		got, explicit := externalDatabaseURL()
		if got != defaultTestDatabaseURL || explicit {
			t.Fatalf("externalDatabaseURL() = %q, %t; want %q, false", got, explicit, defaultTestDatabaseURL)
		}
	})
}
