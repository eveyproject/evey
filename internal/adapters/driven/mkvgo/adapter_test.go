package mkvgo

import (
	"testing"
	"time"

	extmkv "github.com/gravity-zero/mkvgo/mkv"

	"github.com/eveyproject/evey/internal/domain/mkv"
)

func ptr[T any](v T) *T { return &v }

func TestFromExtContainer(t *testing.T) {
	now := time.Now()
	ext := &extmkv.Container{
		DurationMs: 120000,
		Info: extmkv.SegmentInfo{
			Title:         "Test Movie",
			MuxingApp:     "libmatroska",
			WritingApp:    "mkvmerge",
			Duration:      120000,
			TimecodeScale: 1000000,
			DateUTC:       &now,
		},
		Tracks: []extmkv.Track{
			{ID: 1, Type: extmkv.VideoTrack, Codec: "h264", Width: ptr[uint32](1920), Height: ptr[uint32](1080), IsDefault: true},
			{ID: 2, Type: extmkv.AudioTrack, Codec: "aac", Channels: ptr[uint8](6), SampleRate: ptr[float64](48000), BitDepth: ptr[uint8](24), Language: "fre", Name: "French"},
			{ID: 3, Type: extmkv.SubtitleTrack, Codec: "srt", Language: "eng", IsForced: true},
		},
		Chapters: []extmkv.Chapter{
			{ID: 1, Title: "Intro", StartMs: 0, EndMs: 5000},
			{ID: 2, Title: "Main", StartMs: 5000, EndMs: 100000},
		},
		Attachments: []extmkv.Attachment{
			{ID: 1, Name: "cover.jpg", MIMEType: "image/jpeg", Size: 4096, Data: []byte("jpg-data")},
		},
		Tags: []extmkv.Tag{
			{TargetType: "MOVIE", TargetID: 0, SimpleTags: []extmkv.SimpleTag{
				{Name: "TITLE", Value: "Test", Language: "eng"},
			}},
		},
	}

	c := fromExtContainer(ext, "/media/test.mkv")

	if c.Path != "/media/test.mkv" {
		t.Errorf("path = %q, want /media/test.mkv", c.Path)
	}
	if c.Duration.Milliseconds() != 120000 {
		t.Errorf("duration = %d, want 120000", c.Duration.Milliseconds())
	}
	if c.Info.Title != "Test Movie" {
		t.Errorf("title = %q", c.Info.Title)
	}
	if c.Info.DateUTC == nil || !c.Info.DateUTC.Equal(now) {
		t.Error("DateUTC mismatch")
	}

	// Tracks
	if len(c.Tracks) != 3 {
		t.Fatalf("tracks = %d, want 3", len(c.Tracks))
	}
	v := c.Tracks[0]
	if v.Type != mkv.VideoTrack || v.Codec != "h264" || *v.Width != 1920 || *v.Height != 1080 {
		t.Errorf("video track: %+v", v)
	}
	a := c.Tracks[1]
	if a.Type != mkv.AudioTrack || *a.Channels != 6 || a.Language != "fre" || a.Name != "French" {
		t.Errorf("audio track: %+v", a)
	}
	s := c.Tracks[2]
	if s.Type != mkv.SubtitleTrack || !s.IsForced {
		t.Errorf("subtitle track: %+v", s)
	}

	// Chapters
	if len(c.Chapters) != 2 || c.Chapters[0].Title != "Intro" || c.Chapters[1].StartMs != 5000 {
		t.Errorf("chapters: %+v", c.Chapters)
	}

	// Attachments
	if len(c.Attachments) != 1 || c.Attachments[0].Name != "cover.jpg" || c.Attachments[0].Size != 4096 {
		t.Errorf("attachments: %+v", c.Attachments)
	}

	// Tags
	if len(c.Tags) != 1 || c.Tags[0].TargetType != "MOVIE" || len(c.Tags[0].SimpleTag) != 1 {
		t.Errorf("tags: %+v", c.Tags)
	}
	if c.Tags[0].SimpleTag[0].Name != "TITLE" || c.Tags[0].SimpleTag[0].Value != "Test" {
		t.Errorf("simple tag: %+v", c.Tags[0].SimpleTag[0])
	}
}

func TestFromExtTrackNilPointers(t *testing.T) {
	ext := extmkv.Track{ID: 1, Type: extmkv.VideoTrack, Codec: "hevc"}
	out := fromExtTrack(ext)

	if out.Width != nil || out.Height != nil || out.Channels != nil || out.SampleRate != nil || out.BitDepth != nil {
		t.Error("nil pointer fields should remain nil")
	}
}
