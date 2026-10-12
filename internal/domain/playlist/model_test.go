package playlist_test

import (
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/playlist"
)

func TestPlaylistTypeConstants(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ  playlist.PlaylistType
		want string
	}{
		{playlist.Manual, "manual"},
		{playlist.Smart, "smart"},
	}
	for _, c := range cases {
		t.Run(c.want, func(t *testing.T) {
			t.Parallel()
			if string(c.typ) != c.want {
				t.Errorf("PlaylistType = %q, want %q", c.typ, c.want)
			}
		})
	}
}

func TestPlaylist_ManualVsSmart(t *testing.T) {
	t.Parallel()

	now := time.Now()

	t.Run("manual playlist has no rules", func(t *testing.T) {
		t.Parallel()
		p := &playlist.Playlist{
			ID:        "pl1",
			Name:      "My Favorites",
			ProfileID: "u1",
			Type:      playlist.Manual,
			Rules:     nil,
			CreatedAt: now,
			UpdatedAt: now,
		}
		if p.Type != playlist.Manual {
			t.Errorf("Type = %q, want %q", p.Type, playlist.Manual)
		}
		if len(p.Rules) != 0 {
			t.Error("manual playlist should have no rules")
		}
	})

	t.Run("smart playlist has rules", func(t *testing.T) {
		t.Parallel()
		p := &playlist.Playlist{
			ID:        "pl2",
			Name:      "Action Movies",
			ProfileID: "u1",
			Type:      playlist.Smart,
			Rules: []playlist.Rule{
				{Field: "genre", Operator: "contains", Value: "action"},
				{Field: "year", Operator: "gte", Value: "2000"},
			},
			SortBy:    "title",
			SortOrder: "asc",
			CreatedAt: now,
			UpdatedAt: now,
		}
		if p.Type != playlist.Smart {
			t.Errorf("Type = %q, want %q", p.Type, playlist.Smart)
		}
		if len(p.Rules) != 2 {
			t.Errorf("Rules len = %d, want 2", len(p.Rules))
		}
		if p.Rules[0].Field != "genre" {
			t.Errorf("Rules[0].Field = %q, want %q", p.Rules[0].Field, "genre")
		}
		if p.Rules[0].Operator != "contains" {
			t.Errorf("Rules[0].Operator = %q, want %q", p.Rules[0].Operator, "contains")
		}
	})
}

func TestPlaylist_IsOwner(t *testing.T) {
	t.Parallel()
	p := &playlist.Playlist{ProfileID: "u1"}
	if !p.IsOwnedBy("u1") {
		t.Error("owner should be recognized")
	}
	if p.IsOwnedBy("u2") {
		t.Error("non-owner should not be recognized")
	}
}

func TestPlaylist_IsSmart(t *testing.T) {
	t.Parallel()
	manual := &playlist.Playlist{Type: playlist.Manual}
	if manual.IsSmart() {
		t.Error("manual playlist should not be smart")
	}
	smart := &playlist.Playlist{Type: playlist.Smart}
	if !smart.IsSmart() {
		t.Error("smart playlist should be smart")
	}
}

func TestPlaylist_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		pl      playlist.Playlist
		wantErr string
	}{
		{
			name:    "valid manual",
			pl:      playlist.Playlist{Name: "fav", ProfileID: "u1", Type: playlist.Manual},
			wantErr: "",
		},
		{
			name:    "valid smart",
			pl:      playlist.Playlist{Name: "smart", ProfileID: "u1", Type: playlist.Smart},
			wantErr: "",
		},
		{
			name:    "missing name",
			pl:      playlist.Playlist{ProfileID: "u1", Type: playlist.Manual},
			wantErr: "playlist name is required",
		},
		{
			name:    "missing profile ID",
			pl:      playlist.Playlist{Name: "fav", Type: playlist.Manual},
			wantErr: "playlist profile ID is required",
		},
		{
			name:    "invalid type",
			pl:      playlist.Playlist{Name: "fav", ProfileID: "u1", Type: "unknown"},
			wantErr: "playlist type must be 'manual' or 'smart'",
		},
		{
			name:    "empty type",
			pl:      playlist.Playlist{Name: "fav", ProfileID: "u1"},
			wantErr: "playlist type must be 'manual' or 'smart'",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.pl.Validate()
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

func TestNewPlaylist_HappyPath(t *testing.T) {
	t.Parallel()
	p, err := playlist.NewPlaylist("My List", "u1", playlist.Manual)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.ID == "" || p.Name != "My List" || p.ProfileID != "u1" || p.Type != playlist.Manual {
		t.Errorf("unexpected playlist: %+v", p)
	}
	if p.CreatedAt.IsZero() || p.UpdatedAt.IsZero() {
		t.Error("expected timestamps set")
	}
}

func TestNewPlaylist_RejectsEmptyName(t *testing.T) {
	t.Parallel()
	if _, err := playlist.NewPlaylist("", "u1", playlist.Manual); err == nil {
		t.Fatal("expected error for empty name")
	}
}

func TestNewPlaylist_RejectsInvalidType(t *testing.T) {
	t.Parallel()
	if _, err := playlist.NewPlaylist("name", "u1", "bogus"); err == nil {
		t.Fatal("expected error for invalid type")
	}
}

func TestPlaylistItem_SortOrder(t *testing.T) {
	t.Parallel()

	now := time.Now()
	items := []playlist.PlaylistItem{
		{PlaylistID: "pl1", MediaItemID: "m3", SortOrder: 2, AddedAt: now},
		{PlaylistID: "pl1", MediaItemID: "m1", SortOrder: 0, AddedAt: now},
		{PlaylistID: "pl1", MediaItemID: "m2", SortOrder: 1, AddedAt: now},
	}

	// verify sort orders are set distinctly
	seen := map[int]bool{}
	for _, item := range items {
		if seen[item.SortOrder] {
			t.Errorf("duplicate SortOrder %d", item.SortOrder)
		}
		seen[item.SortOrder] = true
	}
}
