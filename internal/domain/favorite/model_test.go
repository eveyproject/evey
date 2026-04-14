package favorite_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/favorite"
)

func TestFavorite_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		fav     favorite.Favorite
		wantErr string
	}{
		{
			name:    "valid",
			fav:     favorite.Favorite{ProfileID: "p1", MediaItemID: "m1"},
			wantErr: "",
		},
		{
			name:    "missing profile ID",
			fav:     favorite.Favorite{MediaItemID: "m1"},
			wantErr: "favorite requires a profile ID",
		},
		{
			name:    "missing media item ID",
			fav:     favorite.Favorite{ProfileID: "p1"},
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

func TestFavoriteEventNames(t *testing.T) {
	t.Parallel()

	cases := []struct {
		event interface{ EventName() string }
		want  string
	}{
		{favorite.FavoriteAdded{}, "favorite.added"},
		{favorite.FavoriteRemoved{}, "favorite.removed"},
	}

	for _, c := range cases {
		got := c.event.EventName()
		if got != c.want {
			t.Errorf("EventName() = %q, want %q", got, c.want)
		}
	}
}
