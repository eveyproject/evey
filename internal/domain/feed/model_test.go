package feed_test

import (
	"testing"

	"github.com/evey-project/evey/internal/domain/feed"
)

func TestType_IsValid(t *testing.T) {
	t.Parallel()

	if feed.Type("").IsValid() {
		t.Error("empty type should be invalid")
	}
	if !feed.TypeRecentlyAdded.IsValid() {
		t.Error("recently_added should be valid")
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
		wantErr string
	}{
		{
			name:    "valid",
			cfg:     feed.AdminConfig{FeedType: feed.TypeRecentlyAdded, MaxItems: 10},
			wantErr: "",
		},
		{
			name:    "empty feed type",
			cfg:     feed.AdminConfig{FeedType: "", MaxItems: 10},
			wantErr: "invalid feed type",
		},
		{
			name:    "negative max items",
			cfg:     feed.AdminConfig{FeedType: feed.TypeRecentlyAdded, MaxItems: -1},
			wantErr: "feed max items must not be negative",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
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

func TestProfileConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     feed.ProfileConfig
		wantErr string
	}{
		{
			name:    "valid",
			cfg:     feed.ProfileConfig{ProfileID: "p1", FeedType: feed.TypeRecentlyAdded},
			wantErr: "",
		},
		{
			name:    "missing profile ID",
			cfg:     feed.ProfileConfig{FeedType: feed.TypeRecentlyAdded},
			wantErr: "feed profile config requires a profile ID",
		},
		{
			name:    "empty feed type",
			cfg:     feed.ProfileConfig{ProfileID: "p1"},
			wantErr: "invalid feed type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
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
