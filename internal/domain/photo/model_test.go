package photo_test

import (
	"strings"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/photo"
)

func TestNewPhoto(t *testing.T) {
	t.Parallel()

	mod := time.Date(2024, 1, 2, 3, 4, 5, 0, time.UTC)
	taken := time.Date(2023, 6, 1, 12, 0, 0, 0, time.UTC)

	t.Run("happy path", func(t *testing.T) {
		t.Parallel()
		p, err := photo.NewPhoto("lib-1", "/photos/x.jpg", 1024, mod,
			photo.WithTitle("X"),
			photo.WithHash("abc"),
			photo.WithMimeType("image/jpeg"),
			photo.WithDimensions(800, 600),
			photo.WithTakenAt(&taken),
			photo.WithAlbum("Summer"),
		)
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		if !strings.HasPrefix(p.ID, "photo-") {
			t.Errorf("ID = %q, want photo-<id>", p.ID)
		}
		if p.LibraryID != "lib-1" || p.FilePath != "/photos/x.jpg" || p.FileSize != 1024 || !p.ModTime.Equal(mod) {
			t.Errorf("required fields mismatched: %+v", p)
		}
		if p.Title != "X" || p.FileHash != "abc" || p.MimeType != "image/jpeg" || p.Width != 800 || p.Height != 600 || p.Album != "Summer" {
			t.Errorf("options not applied: %+v", p)
		}
		if p.TakenAt == nil || !p.TakenAt.Equal(taken) {
			t.Errorf("TakenAt not set correctly: %v", p.TakenAt)
		}
		if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
			t.Errorf("timestamps not stamped: created=%v updated=%v", p.CreatedAt, p.UpdatedAt)
		}
	})

	t.Run("invariant errors bubble up", func(t *testing.T) {
		t.Parallel()
		if _, err := photo.NewPhoto("", "/photos/x.jpg", 1, mod); err == nil {
			t.Fatal("expected library ID error")
		}
		if _, err := photo.NewPhoto("lib-1", "", 1, mod); err == nil {
			t.Fatal("expected file path error")
		}
	})
}

func TestPhoto_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		p       photo.Photo
		wantErr string
	}{
		{
			name:    "valid",
			p:       photo.Photo{LibraryID: "lib1", FilePath: "/photos/img.jpg"},
			wantErr: "",
		},
		{
			name:    "missing library ID",
			p:       photo.Photo{FilePath: "/photos/img.jpg"},
			wantErr: "photo requires a library ID",
		},
		{
			name:    "missing file path",
			p:       photo.Photo{LibraryID: "lib1"},
			wantErr: "photo requires a file path",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.p.Validate()
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
