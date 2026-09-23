package library_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/library"
)

func TestFavorite_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fav     library.Favorite
		wantErr string
	}{
		{
			name:    "valid",
			fav:     library.Favorite{ID: "fav-1", ProfileID: "p1", MediaItemID: "m1"},
			wantErr: "",
		},
		{
			name:    "missing ID",
			fav:     library.Favorite{ProfileID: "p1", MediaItemID: "m1"},
			wantErr: "favorite requires an ID",
		},
		{
			name:    "missing profile ID",
			fav:     library.Favorite{ID: "fav-1", MediaItemID: "m1"},
			wantErr: "favorite requires a profile ID",
		},
		{
			name:    "missing media item ID",
			fav:     library.Favorite{ID: "fav-1", ProfileID: "p1"},
			wantErr: "favorite requires a media item ID",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.fav.Validate()
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

func TestNewFavorite(t *testing.T) {
	t.Parallel()

	t.Run("happy path stamps CreatedAt", func(t *testing.T) {
		t.Parallel()
		f, err := library.NewFavorite("fav-1", "p1", "m1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if f.CreatedAt.IsZero() {
			t.Error("expected CreatedAt to be stamped")
		}
		if f.ID != "fav-1" || f.ProfileID != "p1" || f.MediaItemID != "m1" {
			t.Errorf("fields not wired: %+v", f)
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := library.NewFavorite("", "p1", "m1"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing profile rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := library.NewFavorite("fav-1", "", "m1"); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing media item rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := library.NewFavorite("fav-1", "p1", ""); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
