package music_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/music"
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
			tr:      music.Track{ID: "tr-1", LibraryID: "lib-1", Title: "Song", FilePath: "/music/song.flac"},
			wantErr: "",
		},
		{
			name:    "missing ID",
			tr:      music.Track{LibraryID: "lib-1", Title: "Song", FilePath: "/music/song.flac"},
			wantErr: "track requires an ID",
		},
		{
			name:    "missing library",
			tr:      music.Track{ID: "tr-1", Title: "Song", FilePath: "/music/song.flac"},
			wantErr: "track requires a library ID",
		},
		{
			name:    "missing title",
			tr:      music.Track{ID: "tr-1", LibraryID: "lib-1", FilePath: "/music/song.flac"},
			wantErr: "track requires a title",
		},
		{
			name:    "missing file path",
			tr:      music.Track{ID: "tr-1", LibraryID: "lib-1", Title: "Song"},
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

func TestNewTrack(t *testing.T) {
	t.Parallel()

	t.Run("happy path wires options", func(t *testing.T) {
		t.Parallel()
		tr, err := music.NewTrack("tr-1", "lib-1", "Song", "/m/s.mp3",
			music.WithTrackArtist("Artist", "AlbumArtist"),
			music.WithTrackAlbum("Album"),
			music.WithTrackYear(2025),
			music.WithTrackNumbers(3, 1),
			music.WithTrackFile(1024, "audio/mpeg"),
			music.WithTrackGenre("Rock"),
			music.WithTrackCoverArt("/api/v1/music/tracks/tr-1/art"),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if tr.Album != "Album" || tr.Artist != "Artist" || tr.AlbumArtist != "AlbumArtist" ||
			tr.Year != 2025 || tr.TrackNumber != 3 || tr.DiskNumber != 1 ||
			tr.FileSize != 1024 || tr.MimeType != "audio/mpeg" ||
			tr.Genre != "Rock" || tr.CoverArtURL == "" {
			t.Errorf("fields not wired: %+v", tr)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := music.NewTrack("", "lib-1", "t", "/p"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing library rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := music.NewTrack("tr-1", "", "t", "/p"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
