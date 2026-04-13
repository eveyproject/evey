package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/transcoding"
	tcfakes "github.com/gravity-zero/evey/internal/domain/transcoding/fakes"
)

func newTestTranscodingHandler() (*transcodingHandler, *tcfakes.FakeTranscoder, *tcfakes.FakeTranscodeJobRepository, *tcfakes.FakeTranscodeProfileRepository) {
	transcoder := tcfakes.NewFakeTranscoder()
	hwDetector := tcfakes.NewFakeHWDetector()
	jobRepo := tcfakes.NewFakeTranscodeJobRepository()
	profileRepo := tcfakes.NewFakeTranscodeProfileRepository()
	events := sharedfakes.NewFakeEventBus()
	svc := apptranscoding.NewService(transcoder, hwDetector, jobRepo, profileRepo, events, 5)
	return &transcodingHandler{svc: svc}, transcoder, jobRepo, profileRepo
}

func TestTranscodingGetTranscodeStatus(t *testing.T) {
	h, transcoder, _, _ := newTestTranscodingHandler()
	transcoder.SetProgress("tj-1", &transcoding.TranscodeProgress{
		JobID:   "tj-1",
		Percent: 42.5,
		FPS:     30.0,
		Speed:   1.5,
	})

	resp, err := h.GetTranscodeStatus(context.Background(), &eveyv1.GetTranscodeStatusRequest{JobId: "tj-1"})
	if err != nil {
		t.Fatalf("GetTranscodeStatus: %v", err)
	}
	if resp.Id != "tj-1" {
		t.Errorf("expected job id 'tj-1', got %q", resp.Id)
	}
	if resp.ProgressPercent != 42.5 {
		t.Errorf("expected percent 42.5, got %f", resp.ProgressPercent)
	}
	if resp.Fps != 30.0 {
		t.Errorf("expected fps 30.0, got %f", resp.Fps)
	}
	if resp.Speed != 1.5 {
		t.Errorf("expected speed 1.5, got %f", resp.Speed)
	}
}

func TestTranscodingListActiveTranscodes(t *testing.T) {
	h, _, jobRepo, _ := newTestTranscodingHandler()
	jobRepo.Create(context.Background(), &transcoding.TranscodeJob{
		ID:          "tj-1",
		MediaItemID: "mi-1",
		Status:      transcoding.Running,
		OutputCodec: "h264",
	})
	jobRepo.Create(context.Background(), &transcoding.TranscodeJob{
		ID:          "tj-2",
		MediaItemID: "mi-2",
		Status:      transcoding.Running,
		OutputCodec: "hevc",
	})
	jobRepo.Create(context.Background(), &transcoding.TranscodeJob{
		ID:          "tj-3",
		MediaItemID: "mi-3",
		Status:      transcoding.Completed,
		OutputCodec: "h264",
	})

	resp, err := h.ListActiveTranscodes(context.Background(), &eveyv1.ListActiveTranscodesRequest{})
	if err != nil {
		t.Fatalf("ListActiveTranscodes: %v", err)
	}
	if len(resp.Jobs) != 2 {
		t.Fatalf("expected 2 active jobs, got %d", len(resp.Jobs))
	}
}

func TestTranscodingListActiveTranscodesEmpty(t *testing.T) {
	h, _, _, _ := newTestTranscodingHandler()
	resp, err := h.ListActiveTranscodes(context.Background(), &eveyv1.ListActiveTranscodesRequest{})
	if err != nil {
		t.Fatalf("ListActiveTranscodes: %v", err)
	}
	if len(resp.Jobs) != 0 {
		t.Errorf("expected 0 jobs, got %d", len(resp.Jobs))
	}
}

func TestTranscodingListProfiles(t *testing.T) {
	h, _, _, profileRepo := newTestTranscodingHandler()
	profileRepo.Create(context.Background(), &transcoding.TranscodeProfile{
		ID:           "tp-1",
		Name:         "720p",
		VideoCodec:   "h264",
		AudioCodec:   "aac",
		Container:    "mp4",
		MaxWidth:     1280,
		MaxHeight:    720,
		VideoBitrate: 4000000,
	})

	resp, err := h.ListTranscodeProfiles(context.Background(), &eveyv1.ListTranscodeProfilesRequest{})
	if err != nil {
		t.Fatalf("ListTranscodeProfiles: %v", err)
	}
	if len(resp.Profiles) != 1 {
		t.Fatalf("expected 1 profile, got %d", len(resp.Profiles))
	}
	if resp.Profiles[0].Name != "720p" {
		t.Errorf("expected name '720p', got %q", resp.Profiles[0].Name)
	}
	if resp.Profiles[0].VideoCodec != "h264" {
		t.Errorf("expected codec 'h264', got %q", resp.Profiles[0].VideoCodec)
	}
}
