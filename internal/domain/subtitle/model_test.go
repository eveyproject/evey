package subtitle_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/subtitle"
)

func TestSubtitleDownload_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		dl      subtitle.SubtitleDownload
		wantErr string
	}{
		{
			name:    "valid",
			dl:      subtitle.SubtitleDownload{ID: "sub-1", MediaItemID: "m1", Language: "en", FilePath: "/subs/en.srt"},
			wantErr: "",
		},
		{
			name:    "missing ID",
			dl:      subtitle.SubtitleDownload{MediaItemID: "m1", Language: "en", FilePath: "/subs/en.srt"},
			wantErr: "subtitle download requires an ID",
		},
		{
			name:    "missing media item ID",
			dl:      subtitle.SubtitleDownload{ID: "sub-1", Language: "en", FilePath: "/subs/en.srt"},
			wantErr: "subtitle download requires a media item ID",
		},
		{
			name:    "missing language",
			dl:      subtitle.SubtitleDownload{ID: "sub-1", MediaItemID: "m1", FilePath: "/subs/en.srt"},
			wantErr: "subtitle download requires a language",
		},
		{
			name:    "missing file path",
			dl:      subtitle.SubtitleDownload{ID: "sub-1", MediaItemID: "m1", Language: "en"},
			wantErr: "subtitle download requires a file path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.dl.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}

func TestNewSubtitleDownload(t *testing.T) {
	t.Parallel()

	t.Run("happy path stamps DownloadedAt", func(t *testing.T) {
		t.Parallel()
		d, err := subtitle.NewSubtitleDownload("sub-1", "m1", "en", "/subs/en.srt",
			subtitle.WithSubtitleFormat("srt"),
			subtitle.WithSubtitleSource("opensubtitles"),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if d.DownloadedAt.IsZero() || d.Format != "srt" || d.Source != "opensubtitles" {
			t.Errorf("fields not wired: %+v", d)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := subtitle.NewSubtitleDownload("", "m1", "en", "/x"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing media rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := subtitle.NewSubtitleDownload("sub-1", "", "en", "/x"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestSearchResult_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		sr      subtitle.SearchResult
		wantErr string
	}{
		{
			name:    "valid",
			sr:      subtitle.SearchResult{FileName: "sub.srt", Language: "en"},
			wantErr: "",
		},
		{
			name:    "missing file name",
			sr:      subtitle.SearchResult{Language: "en"},
			wantErr: "search result requires a file name",
		},
		{
			name:    "missing language",
			sr:      subtitle.SearchResult{FileName: "sub.srt"},
			wantErr: "search result requires a language",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.sr.Validate()
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if err.Error() != tt.wantErr {
				t.Errorf("error = %q, want %q", err.Error(), tt.wantErr)
			}
		})
	}
}
