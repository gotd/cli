package main

import (
	"testing"

	"github.com/gotd/td/telegram/message"
	"github.com/gotd/td/tg"
)

func TestAlbumMediaKinds(t *testing.T) {
	file := &tg.InputFile{ID: 1, Name: "x"}
	for _, tc := range []struct {
		mime string
		want any
	}{
		{"image/jpeg", (*message.UploadedPhotoBuilder)(nil)},
		{"video/mp4", (*message.VideoDocumentBuilder)(nil)},
		{"audio/mpeg", (*message.AudioDocumentBuilder)(nil)},
		{"application/pdf", (*message.UploadedDocumentBuilder)(nil)},
	} {
		got := albumMedia(file, "f", tc.mime, nil)
		switch tc.want.(type) {
		case *message.UploadedPhotoBuilder:
			if _, ok := got.(*message.UploadedPhotoBuilder); !ok {
				t.Errorf("%s -> %T, want photo", tc.mime, got)
			}
		case *message.VideoDocumentBuilder:
			if _, ok := got.(*message.VideoDocumentBuilder); !ok {
				t.Errorf("%s -> %T, want video", tc.mime, got)
			}
		case *message.AudioDocumentBuilder:
			if _, ok := got.(*message.AudioDocumentBuilder); !ok {
				t.Errorf("%s -> %T, want audio", tc.mime, got)
			}
		case *message.UploadedDocumentBuilder:
			if _, ok := got.(*message.UploadedDocumentBuilder); !ok {
				t.Errorf("%s -> %T, want document", tc.mime, got)
			}
		}
	}
}
