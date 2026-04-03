package music_test

import (
	"testing"

	"github.com/gravity-zero/evey/internal/domain/music"
)

func TestTrack_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		tr      music.Track
		wantErr string
	}{
		{
			name:    "valid",
			tr:      music.Track{Title: "Song", FilePath: "/music/song.flac"},
			wantErr: "",
		},
		{
			name:    "missing title",
			tr:      music.Track{FilePath: "/music/song.flac"},
			wantErr: "track requires a title",
		},
		{
			name:    "missing file path",
			tr:      music.Track{Title: "Song"},
			wantErr: "track requires a file path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.tr.Validate()
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
