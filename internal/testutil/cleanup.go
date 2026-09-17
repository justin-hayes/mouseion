package testutil

type cleanupT interface {
	Helper()
	Cleanup(func())
	Errorf(string, ...any)
}

// Cleanup registers an error-returning cleanup without replacing an earlier
// test failure with cleanup noise.
func Cleanup(t cleanupT, name string, cleanup func() error) {
	t.Helper()
	t.Cleanup(func() {
		if err := cleanup(); err != nil {
			t.Errorf("%s cleanup failed: %v", name, err)
		}
	})
}
