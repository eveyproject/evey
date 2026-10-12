package feed_test

import (
	"errors"
	"testing"

	"github.com/eveyproject/evey/internal/domain/feed"
)

func TestType_IsValid(t *testing.T) {
	t.Parallel()

	if feed.Type("").IsValid() {
		t.Error("empty type should be invalid")
	}
	if !feed.TypeRecentlyAdded.IsValid() {
		t.Error("recently_added should be valid")
	}
	if feed.Type("reccently_added").IsValid() {
		t.Error("a typo'd/unknown built-in type must be invalid, not accepted as any non-empty string")
	}
	if !feed.LibraryType("lib-1").IsValid() {
		t.Error("a well-formed library:<id> type should be valid")
	}
	if feed.Type("library:").IsValid() {
		t.Error("library: with no id must be invalid")
	}
}

func TestType_IsPlugin(t *testing.T) {
	t.Parallel()

	pl := feed.PluginType("plg-abc123")
	if !pl.IsPlugin() {
		t.Error("plugin type should return true for IsPlugin")
	}
	if pl.PluginID() != "plg-abc123" {
		t.Errorf("PluginID() = %q, want %q", pl.PluginID(), "plg-abc123")
	}
	if feed.TypeRecentlyAdded.IsPlugin() {
		t.Error("non-plugin type should return false")
	}
	if feed.TypeRecentlyAdded.PluginID() != "" {
		t.Error("non-plugin type should return empty PluginID")
	}
	// Library and plugin types are disjoint.
	if feed.LibraryType("lib-1").IsPlugin() {
		t.Error("library type must not be detected as plugin")
	}
	if pl.IsLibrary() {
		t.Error("plugin type must not be detected as library")
	}
}

func TestType_IsLibrary(t *testing.T) {
	t.Parallel()

	lib := feed.LibraryType("lib-1")
	if !lib.IsLibrary() {
		t.Error("library type should return true for IsLibrary")
	}
	if lib.LibraryID() != "lib-1" {
		t.Errorf("LibraryID() = %q, want %q", lib.LibraryID(), "lib-1")
	}
	if feed.TypeRecentlyAdded.IsLibrary() {
		t.Error("non-library type should return false")
	}
	if feed.TypeRecentlyAdded.LibraryID() != "" {
		t.Error("non-library type should return empty LibraryID")
	}
}

func TestAdminConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     feed.AdminConfig
		wantErr error
	}{
		{
			name:    "valid",
			cfg:     feed.AdminConfig{FeedType: feed.TypeRecentlyAdded, MaxItems: 10},
			wantErr: nil,
		},
		{
			name:    "empty feed type",
			cfg:     feed.AdminConfig{FeedType: "", MaxItems: 10},
			wantErr: feed.ErrInvalidFeedType,
		},
		{
			name:    "negative max items",
			cfg:     feed.AdminConfig{FeedType: feed.TypeRecentlyAdded, MaxItems: -1},
			wantErr: feed.ErrNegativeMaxItems,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestProfileConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     feed.ProfileConfig
		wantErr error
	}{
		{
			name:    "valid",
			cfg:     feed.ProfileConfig{ProfileID: "p1", FeedType: feed.TypeRecentlyAdded},
			wantErr: nil,
		},
		{
			name:    "missing profile ID",
			cfg:     feed.ProfileConfig{FeedType: feed.TypeRecentlyAdded},
			wantErr: feed.ErrInvalidProfileID,
		},
		{
			name:    "empty feed type",
			cfg:     feed.ProfileConfig{ProfileID: "p1"},
			wantErr: feed.ErrInvalidFeedType,
		},
		{
			name: "negative max items override",
			cfg: feed.ProfileConfig{ProfileID: "p1", FeedType: feed.TypeRecentlyAdded,
				MaxItems: func() *int { v := -1; return &v }()},
			wantErr: feed.ErrNegativeMaxItems,
		},
		{
			name: "zero max items override is valid",
			cfg: feed.ProfileConfig{ProfileID: "p1", FeedType: feed.TypeRecentlyAdded,
				MaxItems: func() *int { v := 0; return &v }()},
			wantErr: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error = %v, want %v", err, tt.wantErr)
			}
		})
	}
}

func TestNewFeedAdminConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid no options", func(t *testing.T) {
		t.Parallel()
		cfg, err := feed.NewFeedAdminConfig(feed.TypeRecentlyAdded)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.FeedType != feed.TypeRecentlyAdded {
			t.Errorf("FeedType = %q, want %q", cfg.FeedType, feed.TypeRecentlyAdded)
		}
		if cfg.Enabled || cfg.DisplayOrder != 0 || cfg.MaxItems != 0 {
			t.Errorf("expected zero-value defaults, got %+v", cfg)
		}
	})

	t.Run("valid with options", func(t *testing.T) {
		t.Parallel()
		params := map[string]any{"sort": "added"}
		cfg, err := feed.NewFeedAdminConfig(
			feed.TypeMostPopular,
			feed.WithAdminEnabled(true),
			feed.WithAdminDisplayOrder(3),
			feed.WithAdminMaxItems(25),
			feed.WithAdminParams(params),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !cfg.Enabled {
			t.Error("expected Enabled true")
		}
		if cfg.DisplayOrder != 3 {
			t.Errorf("DisplayOrder = %d, want 3", cfg.DisplayOrder)
		}
		if cfg.MaxItems != 25 {
			t.Errorf("MaxItems = %d, want 25", cfg.MaxItems)
		}
		if got := cfg.Params["sort"]; got != "added" {
			t.Errorf("Params[sort] = %v, want added", got)
		}
		// Defensive clone: mutating caller map must not affect cfg.
		params["sort"] = "tampered"
		if cfg.Params["sort"] != "added" {
			t.Error("WithAdminParams did not clone the input map")
		}
	})

	t.Run("invalid feed type rejected via IsValid", func(t *testing.T) {
		t.Parallel()
		_, err := feed.NewFeedAdminConfig(feed.Type(""))
		if !errors.Is(err, feed.ErrInvalidFeedType) {
			t.Fatalf("error = %v, want ErrInvalidFeedType", err)
		}
	})

	t.Run("negative max items rejected", func(t *testing.T) {
		t.Parallel()
		_, err := feed.NewFeedAdminConfig(
			feed.TypeRecentlyAdded,
			feed.WithAdminMaxItems(-1),
		)
		if !errors.Is(err, feed.ErrNegativeMaxItems) {
			t.Fatalf("error = %v, want ErrNegativeMaxItems", err)
		}
	})
}

func TestNewFeedProfileConfig(t *testing.T) {
	t.Parallel()

	t.Run("valid no options", func(t *testing.T) {
		t.Parallel()
		cfg, err := feed.NewFeedProfileConfig(feed.TypeRecentlyAdded, "p1")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.ProfileID != "p1" {
			t.Errorf("ProfileID = %q, want p1", cfg.ProfileID)
		}
		if cfg.FeedType != feed.TypeRecentlyAdded {
			t.Errorf("FeedType = %q, want %q", cfg.FeedType, feed.TypeRecentlyAdded)
		}
		if cfg.Enabled != nil || cfg.DisplayOrder != nil || cfg.MaxItems != nil {
			t.Errorf("expected nil overrides by default, got %+v", cfg)
		}
	})

	t.Run("valid with options", func(t *testing.T) {
		t.Parallel()
		params := map[string]any{"genre": "drama"}
		cfg, err := feed.NewFeedProfileConfig(
			feed.TypeNextEpisode,
			"p1",
			feed.WithProfileEnabled(false),
			feed.WithProfileDisplayOrder(7),
			feed.WithProfileMaxItems(12),
			feed.WithProfileParams(params),
		)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if cfg.Enabled == nil || *cfg.Enabled {
			t.Errorf("Enabled = %v, want pointer to false", cfg.Enabled)
		}
		if cfg.DisplayOrder == nil || *cfg.DisplayOrder != 7 {
			t.Errorf("DisplayOrder = %v, want pointer to 7", cfg.DisplayOrder)
		}
		if cfg.MaxItems == nil || *cfg.MaxItems != 12 {
			t.Errorf("MaxItems = %v, want pointer to 12", cfg.MaxItems)
		}
		if got := cfg.Params["genre"]; got != "drama" {
			t.Errorf("Params[genre] = %v, want drama", got)
		}
		params["genre"] = "tampered"
		if cfg.Params["genre"] != "drama" {
			t.Error("WithProfileParams did not clone the input map")
		}
	})

	t.Run("missing profile ID rejected", func(t *testing.T) {
		t.Parallel()
		_, err := feed.NewFeedProfileConfig(feed.TypeRecentlyAdded, "")
		if !errors.Is(err, feed.ErrInvalidProfileID) {
			t.Fatalf("error = %v, want ErrInvalidProfileID", err)
		}
	})

	t.Run("invalid feed type rejected via IsValid", func(t *testing.T) {
		t.Parallel()
		_, err := feed.NewFeedProfileConfig(feed.Type(""), "p1")
		if !errors.Is(err, feed.ErrInvalidFeedType) {
			t.Fatalf("error = %v, want ErrInvalidFeedType", err)
		}
	})
}
