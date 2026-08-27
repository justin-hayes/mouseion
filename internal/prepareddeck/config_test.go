package prepareddeck

import (
	"os"
	"testing"
)

func TestTranslationConcurrencyFromEnv(t *testing.T) {
	t.Run("default", func(t *testing.T) {
		previous, existed := os.LookupEnv(TranslationConcurrencyEnv)
		if err := os.Unsetenv(TranslationConcurrencyEnv); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(TranslationConcurrencyEnv, previous)
			}
		})
		got, err := TranslationConcurrencyFromEnv()
		if err != nil || got != DefaultTranslationConcurrency {
			t.Fatalf("TranslationConcurrencyFromEnv() = %d, %v; want %d, nil", got, err, DefaultTranslationConcurrency)
		}
	})

	t.Run("custom", func(t *testing.T) {
		t.Setenv(TranslationConcurrencyEnv, "4")
		got, err := TranslationConcurrencyFromEnv()
		if err != nil || got != 4 {
			t.Fatalf("TranslationConcurrencyFromEnv() = %d, %v; want 4, nil", got, err)
		}
	})

	for _, value := range []string{"", "invalid", "0", "-1", "1.5"} {
		t.Run("invalid_"+value, func(t *testing.T) {
			t.Setenv(TranslationConcurrencyEnv, value)
			if _, err := TranslationConcurrencyFromEnv(); err == nil {
				t.Fatalf("TranslationConcurrencyFromEnv() with %q returned nil error", value)
			}
		})
	}
}
