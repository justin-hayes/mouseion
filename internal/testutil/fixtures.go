package testutil

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"path/filepath"
	"testing"
)

// ZipDirectory packages a checked-in fixture directory as an in-memory ZIP.
func ZipDirectory(t *testing.T, directory string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(directory, func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		rel, err := filepath.Rel(directory, name)
		if err != nil {
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		data, err := os.ReadFile(name) //nolint:gosec // filepath.WalkDir confines this to the checked-in fixture directory.
		if err != nil {
			return err
		}
		_, err = w.Write(data)
		return err
	})
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}
