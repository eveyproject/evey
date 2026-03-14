package playlist_test

import (
	"testing"
	"time"

	"github.com/gravity-zero/evey/internal/domain/playlist"
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
			UserID:    "u1",
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
			ID:     "pl2",
			Name:   "Action Movies",
			UserID: "u1",
			Type:   playlist.Smart,
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
