package webhook_test

import (
	"testing"

	"github.com/eveyproject/evey/internal/domain/webhook"
)

func TestConfig_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     webhook.Config
		wantErr string
	}{
		{
			name:    "empty ID",
			cfg:     webhook.Config{Name: "hook", URL: "http://example.com", Events: []string{"a"}},
			wantErr: "ID is required",
		},
		{
			name:    "empty name",
			cfg:     webhook.Config{ID: "wh-1", Name: "", URL: "http://example.com", Events: []string{"a"}},
			wantErr: "name is required",
		},
		{
			name:    "empty URL",
			cfg:     webhook.Config{ID: "wh-1", Name: "hook", URL: "", Events: []string{"a"}},
			wantErr: "URL is required",
		},
		{
			name:    "no events",
			cfg:     webhook.Config{ID: "wh-1", Name: "hook", URL: "http://example.com", Events: nil},
			wantErr: "at least one event",
		},
		{
			name:    "empty events slice",
			cfg:     webhook.Config{ID: "wh-1", Name: "hook", URL: "http://example.com", Events: []string{}},
			wantErr: "at least one event",
		},
		{
			name: "valid config",
			cfg:  webhook.Config{ID: "wh-1", Name: "hook", URL: "http://example.com", Events: []string{"playback.started"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.cfg.Validate()
			if tt.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tt.wantErr)
				}
				if !containsStr(err.Error(), tt.wantErr) {
					t.Fatalf("expected error containing %q, got %q", tt.wantErr, err.Error())
				}
			} else {
				if err != nil {
					t.Fatalf("expected no error, got %v", err)
				}
			}
		})
	}
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestConfig_MatchesEvent(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name   string
		events []string
		event  string
		want   bool
	}{
		{"wildcard matches any", []string{"*"}, "playback.started", true},
		{"exact match", []string{"playback.started"}, "playback.started", true},
		{"no match", []string{"playback.started"}, "playback.stopped", false},
		{"one of many", []string{"a", "b", "c"}, "b", true},
		{"empty events", nil, "any", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			cfg := &webhook.Config{Events: tt.events}
			if got := cfg.MatchesEvent(tt.event); got != tt.want {
				t.Errorf("MatchesEvent(%q) = %v, want %v", tt.event, got, tt.want)
			}
		})
	}
}

func TestNewConfig(t *testing.T) {
	t.Parallel()

	t.Run("happy path stamps CreatedAt + clones slices/maps", func(t *testing.T) {
		t.Parallel()
		events := []string{"playback.started"}
		headers := map[string]string{"X-Ping": "pong"}
		c, err := webhook.NewConfig("wh-1", "hook", "http://example.com", events,
			webhook.WithConfigSecret("s"),
			webhook.WithConfigHeaders(headers),
			webhook.WithConfigEnabled(true),
		)
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if c.CreatedAt.IsZero() {
			t.Error("expected CreatedAt stamped")
		}
		if !c.Enabled || c.Secret != "s" {
			t.Errorf("options not applied: %+v", c)
		}
		// mutate caller's slice/map: config must not observe the change
		events[0] = "mutated"
		headers["X-Ping"] = "mutated"
		if c.Events[0] != "playback.started" {
			t.Error("Events was not cloned")
		}
		if c.Headers["X-Ping"] != "pong" {
			t.Error("Headers was not cloned")
		}
	})

	t.Run("missing id rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := webhook.NewConfig("", "hook", "http://example.com", []string{"a"}); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("missing events rejected", func(t *testing.T) {
		t.Parallel()
		if _, err := webhook.NewConfig("wh-1", "hook", "http://example.com", nil); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}
