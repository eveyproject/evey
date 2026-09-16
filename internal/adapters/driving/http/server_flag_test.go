package httpapi

import (
	"context"
	"errors"
	"testing"

	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
)

// Operator flags used to be compared against the literal strings "on"/"off",
// while the admin panel persists every other boolean with strconv.FormatBool
// ("true"/"false"). The two never met:
//   - transcoding.hls_ondemand = "false"  did NOT disable it ("false" != "off")
//   - transcoding.hls_ondemand_concat = "true" did NOT enable it ("true" != "on")
//
// An operator could believe a path was off while it ran, or on while it was
// ignored. Accept the usual spellings, and keep the unset value on the default.
func TestFlagEnabled(t *testing.T) {
	for _, tc := range []struct {
		raw       string
		defaultOn bool
		want      bool
	}{
		// unset → the code's default
		{"", true, true},
		{"", false, false},
		// the panel's own format (strconv.FormatBool) must work
		{"true", false, true},
		{"false", true, false},
		// the historical spelling must keep working
		{"on", false, true},
		{"off", true, false},
		// common operator spellings
		{"1", false, true},
		{"0", true, false},
		{"yes", false, true},
		{"no", true, false},
		{" TRUE ", false, true}, // whitespace + case
		{"Off", true, false},
		// garbage must not silently flip a path: fall back to the default
		{"maybe", true, true},
		{"maybe", false, false},
	} {
		if got := flagEnabled(tc.raw, tc.defaultOn); got != tc.want {
			t.Errorf("flagEnabled(%q, default=%v) = %v, want %v", tc.raw, tc.defaultOn, got, tc.want)
		}
	}
}

// Precedence of an operator flag: the server_config row (dotted key, then the
// raw env-var name as a key) beats the process env, which beats the default. A
// blank row is "unset", not "off", and no repository at all is pure env.
func TestResolveServerFlag_Precedence(t *testing.T) {
	const dbKey, envVar = "transcoding.test_flag", "EVEY_TEST_FLAG"
	ctx := context.Background()
	cfg := sharedfakes.NewFakeServerConfigRepository()

	t.Setenv(envVar, "")
	if got := resolveServerFlag(nil, dbKey, envVar); got != "" {
		t.Errorf("nothing set: %q", got)
	}
	if !serverFlagEnabled(cfg, dbKey, envVar, true) || serverFlagEnabled(cfg, dbKey, envVar, false) {
		t.Error("nothing set must keep the default")
	}

	t.Setenv(envVar, "false")
	if serverFlagEnabled(cfg, dbKey, envVar, true) {
		t.Error("env must turn the flag off when no row exists")
	}
	if serverFlagEnabled(nil, dbKey, envVar, true) {
		t.Error("a nil repository must still honour the env")
	}

	if err := cfg.Set(ctx, envVar, "true"); err != nil {
		t.Fatal(err)
	}
	if !serverFlagEnabled(cfg, dbKey, envVar, false) {
		t.Error("a row under the raw env-var name must beat the env")
	}
	if err := cfg.Set(ctx, dbKey, "off"); err != nil {
		t.Fatal(err)
	}
	if serverFlagEnabled(cfg, dbKey, envVar, true) {
		t.Error("the dotted key must win over the env-var-named row")
	}

	if err := cfg.Set(ctx, dbKey, ""); err != nil {
		t.Fatal(err)
	}
	if err := cfg.Delete(ctx, envVar); err != nil {
		t.Fatal(err)
	}
	if got := resolveServerFlag(cfg, dbKey, envVar); got != "false" {
		t.Errorf("a blank row is unset and falls through to the env, got %q", got)
	}

	cfg.Err = errors.New("db down")
	if got := resolveServerFlag(cfg, dbKey, envVar); got != "false" {
		t.Errorf("an unreadable repository falls back to the env, got %q", got)
	}
}
