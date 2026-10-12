package testutil

import (
	"archive/zip"
	"bytes"
	"io/fs"
	"os"
	"testing"
)

// ZipDirectory packages a checked-in fixture directory as an in-memory ZIP.
func ZipDirectory(t *testing.T, directory string) []byte {
	t.Helper()
	root, err := os.OpenRoot(directory)
	if err != nil {
		t.Fatal(err)
	}
	Cleanup(t, "fixture root", root.Close)
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err = fs.WalkDir(root.FS(), ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() {
			return err
		}
		w, err := zw.Create(name)
		if err != nil {
			return err
		}
		data, err := root.ReadFile(name)
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
