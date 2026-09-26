package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appmkv "github.com/evey-project/evey/internal/app/mkv"
	"github.com/evey-project/evey/internal/domain/mkv"
	mkvfakes "github.com/evey-project/evey/internal/domain/mkv/fakes"
	"github.com/evey-project/evey/internal/domain/shared"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestMKVHandler() (*mkvHandler, *mkvfakes.FakeContainerReader) {
	reader := mkvfakes.NewFakeContainerReader()
	svc := appmkv.NewService(reader)
	return &mkvHandler{svc: svc, userAccess: newPermissiveTestUserAccess()}, reader
}

func seedContainer(reader *mkvfakes.FakeContainerReader) {
	w := uint32(1920)
	ht := uint32(1080)
	ch := uint8(6)
	sr := float64(48000)
	dur, _ := shared.NewDurationMs(7200000)
	reader.Containers["/media/movie.mkv"] = &mkv.Container{
		Path:     "/media/movie.mkv",
		Duration: dur,
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
	h, reader := newTestMKVHandler()
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

func TestMKVListChaptersNotFound(t *testing.T) {
	h, _ := newTestMKVHandler()
	_, err := h.ListChapters(context.Background(), &eveyv1.ListChaptersRequest{Path: "/nonexistent.mkv"})
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// The MKV toolbox RPCs had no client and took free disk paths (mux, demux,
// add/remove track, extract): they answer Unimplemented, even to an admin.
func TestMKVToolboxRPCsAreNotExposed(t *testing.T) {
	h, _ := newTestMKVHandler()
	ctx := context.Background()
	calls := map[string]func() error{
		"GetContainerInfo": func() error {
			_, err := h.GetContainerInfo(ctx, &eveyv1.GetContainerInfoRequest{Path: "/media/movie.mkv"})
			return err
		},
		"ListTracks": func() error {
			_, err := h.ListTracks(ctx, &eveyv1.ListMkvTracksRequest{Path: "/media/movie.mkv"})
			return err
		},
		"ListAttachments": func() error {
			_, err := h.ListAttachments(ctx, &eveyv1.ListAttachmentsRequest{Path: "/media/movie.mkv"})
			return err
		},
		"ExtractAttachment": func() error {
			_, err := h.ExtractAttachment(ctx, &eveyv1.ExtractAttachmentRequest{Path: "/media/movie.mkv"})
			return err
		},
		"AddTrack":       func() error { _, err := h.AddTrack(ctx, &eveyv1.AddTrackRequest{}); return err },
		"RemoveTrack":    func() error { _, err := h.RemoveTrack(ctx, &eveyv1.RemoveTrackRequest{}); return err },
		"ExtractTrack":   func() error { return h.ExtractTrack(&eveyv1.ExtractTrackRequest{}, nil) },
		"MuxContainer":   func() error { return h.MuxContainer(&eveyv1.MuxContainerRequest{}, nil) },
		"DemuxContainer": func() error { return h.DemuxContainer(&eveyv1.DemuxContainerRequest{}, nil) },
	}
	for name, call := range calls {
		if got := status.Code(call()); got != codes.Unimplemented {
			t.Errorf("%s: code = %v, want Unimplemented", name, got)
		}
	}
}
