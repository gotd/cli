package main

import (
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gotd/td/tg"
)

func TestMediaLocationDocument(t *testing.T) {
	media := &tg.MessageMediaDocument{}
	media.Document = &tg.Document{
		ID:            5,
		AccessHash:    7,
		FileReference: []byte{1, 2},
		Attributes: []tg.DocumentAttributeClass{
			&tg.DocumentAttributeFilename{FileName: "report.pdf"},
		},
	}

	loc, name, err := mediaLocation(media)
	if err != nil {
		t.Fatal(err)
	}
	if name != "report.pdf" {
		t.Errorf("name = %q, want report.pdf", name)
	}
	if _, ok := loc.(*tg.InputDocumentFileLocation); !ok {
		t.Errorf("loc type = %T, want InputDocumentFileLocation", loc)
	}
}

func TestMediaLocationPhoto(t *testing.T) {
	media := &tg.MessageMediaPhoto{}
	media.Photo = &tg.Photo{
		ID:         9,
		AccessHash: 1,
		Sizes: []tg.PhotoSizeClass{
			&tg.PhotoSize{Type: "m", W: 100, H: 100},
			&tg.PhotoSize{Type: "y", W: 1000, H: 1000},
		},
	}

	loc, name, err := mediaLocation(media)
	if err != nil {
		t.Fatal(err)
	}
	if name != "photo_9.jpg" {
		t.Errorf("name = %q", name)
	}
	pl, ok := loc.(*tg.InputPhotoFileLocation)
	if !ok {
		t.Fatalf("loc type = %T", loc)
	}
	if pl.ThumbSize != "y" {
		t.Errorf("picked size %q, want largest 'y'", pl.ThumbSize)
	}
}

func TestMediaLocationNone(t *testing.T) {
	if _, _, err := mediaLocation(&tg.MessageMediaEmpty{}); err == nil {
		t.Error("expected error for empty media")
	}
}

func TestDocumentFilenameFallback(t *testing.T) {
	doc := &tg.Document{ID: 123}
	if got := documentFilename(doc); got != "document_123" {
		t.Errorf("fallback = %q", got)
	}
}

func TestResolveOutPath(t *testing.T) {
	if got := resolveOutPath("", "a.bin"); got != "a.bin" {
		t.Errorf("default = %q", got)
	}
	if got := resolveOutPath("file.out", "a.bin"); got != "file.out" {
		t.Errorf("explicit file = %q", got)
	}
	dir := t.TempDir()
	if got := resolveOutPath(dir, "a.bin"); got != filepath.Join(dir, "a.bin") {
		t.Errorf("dir join = %q", got)
	}
}

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
