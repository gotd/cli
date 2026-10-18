package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWalkFiles(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.txt", "sub/b.txt"} {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var got []string
	if err := walkFiles(dir, func(path string, _ fs.FileInfo) error {
		got = append(got, filepath.Base(path))
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if strings.Join(got, ",") != "a.txt,b.txt" {
		t.Errorf("visited %v", got)
	}

	// Regression: a missing path used to panic on a nil FileInfo.
	if err := walkFiles(filepath.Join(dir, "missing"), func(string, fs.FileInfo) error { return nil }); err == nil {
		t.Error("expected an error for a missing path")
	}
}
