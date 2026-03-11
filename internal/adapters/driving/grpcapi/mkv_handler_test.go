package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appmkv "github.com/gravity-zero/evey/internal/app/mkv"
	"github.com/gravity-zero/evey/internal/domain/mkv"
	mkvfakes "github.com/gravity-zero/evey/internal/domain/mkv/fakes"
)

func newTestMKVHandler() (*mkvHandler, *mkvfakes.FakeContainerReader, *mkvfakes.FakeContainerWriter) {
	reader := mkvfakes.NewFakeContainerReader()
	writer := mkvfakes.NewFakeContainerWriter()
	svc := appmkv.NewService(reader, writer)
	return &mkvHandler{svc: svc}, reader, writer
}

func seedContainer(reader *mkvfakes.FakeContainerReader) {
	w := uint32(1920)
	ht := uint32(1080)
	ch := uint8(6)
	sr := float64(48000)
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Path:       "/media/movie.mkv",
		DurationMs: 7200000,
		Info: mkv.SegmentInfo{
			Title:      "Test Movie",
			MuxingApp:  "libebml",
			WritingApp: "mkvmerge",
		},
		Tracks: []mkv.Track{
			{ID: 1, Type: mkv.VideoTrack, Codec: "V_MPEG4/ISO/AVC", Language: "und", IsDefault: true, Width: &w, Height: &ht},
			{ID: 2, Type: mkv.AudioTrack, Codec: "A_AAC", Language: "eng", IsDefault: true, Channels: &ch, SampleRate: &sr},
			{ID: 3, Type: mkv.SubtitleTrack, Codec: "S_TEXT/UTF8", Language: "fre", Name: "French"},
		},
		Chapters: []mkv.Chapter{
			{ID: 1, Title: "Intro", StartMs: 0, EndMs: 60000},
			{ID: 2, Title: "Main", StartMs: 60000, EndMs: 7000000},
		},
		Attachments: []mkv.Attachment{
			{ID: 1, Name: "cover.jpg", MIMEType: "image/jpeg", Size: 50000},
		},
	}
}

func TestMKVListChapters(t *testing.T) {
	h, reader, _ := newTestMKVHandler()
	seedContainer(reader)

	resp, err := h.ListChapters(context.Background(), &eveyv1.ListChaptersRequest{Path: "/media/movie.mkv"})
	if err != nil {
		t.Fatalf("ListChapters: %v", err)
	}
	if len(resp.Chapters) != 2 {
		t.Fatalf("expected 2 chapters, got %d", len(resp.Chapters))
	}
	if resp.Chapters[0].Title != "Intro" {
		t.Errorf("expected chapter title 'Intro', got %q", resp.Chapters[0].Title)
	}
	if resp.Chapters[1].StartMs != 60000 {
		t.Errorf("expected start_ms 60000, got %d", resp.Chapters[1].StartMs)
	}
}

func TestMKVListTracks(t *testing.T) {
	h, reader, _ := newTestMKVHandler()
	seedContainer(reader)

	resp, err := h.ListTracks(context.Background(), &eveyv1.ListTracksRequest{Path: "/media/movie.mkv"})
	if err != nil {
		t.Fatalf("ListTracks: %v", err)
	}
	if len(resp.Tracks) != 3 {
		t.Fatalf("expected 3 tracks, got %d", len(resp.Tracks))
	}
	var hasVideo, hasAudio, hasSub bool
	for _, tr := range resp.Tracks {
		switch tr.Type {
		case eveyv1.TrackType_TRACK_TYPE_VIDEO:
			hasVideo = true
			if tr.Width != 1920 || tr.Height != 1080 {
				t.Errorf("expected 1920x1080, got %dx%d", tr.Width, tr.Height)
			}
		case eveyv1.TrackType_TRACK_TYPE_AUDIO:
			hasAudio = true
		case eveyv1.TrackType_TRACK_TYPE_SUBTITLE:
			hasSub = true
		}
	}
	if !hasVideo || !hasAudio || !hasSub {
		t.Errorf("missing tracks: video=%v audio=%v sub=%v", hasVideo, hasAudio, hasSub)
	}
}

func TestMKVListChaptersNotFound(t *testing.T) {
	h, _, _ := newTestMKVHandler()
	_, err := h.ListChapters(context.Background(), &eveyv1.ListChaptersRequest{Path: "/nonexistent.mkv"})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestMKVListAttachments(t *testing.T) {
	h, reader, _ := newTestMKVHandler()
	seedContainer(reader)

	resp, err := h.ListAttachments(context.Background(), &eveyv1.ListAttachmentsRequest{Path: "/media/movie.mkv"})
	if err != nil {
		t.Fatalf("ListAttachments: %v", err)
	}
	if len(resp.Attachments) != 1 {
		t.Fatalf("expected 1 attachment, got %d", len(resp.Attachments))
	}
	if resp.Attachments[0].Name != "cover.jpg" {
		t.Errorf("expected name 'cover.jpg', got %q", resp.Attachments[0].Name)
	}
	if resp.Attachments[0].MimeType != "image/jpeg" {
		t.Errorf("expected mime 'image/jpeg', got %q", resp.Attachments[0].MimeType)
	}
}
