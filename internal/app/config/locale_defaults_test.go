package config

import (
	"context"
	"testing"

	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// The defaults shown in the panel are the ones the server applies: the
// metadata language follows the server language once set, and the zone
// follows TZ.
func TestLocaleDefaults_FollowTheServersOwnChains(t *testing.T) {
	ctx := context.Background()
	repo := sharedfakes.NewFakeServerConfigRepository()
	t.Setenv("TZ", "America/Montreal")

	d := LocaleDefaults(ctx, repo, "fr")
	if d["server.language"] != "fr" || d["metadata.language"] != "fr" || d[KeyServerTimezone] != "America/Montreal" {
		t.Errorf("nothing set: %v", d)
	}
	_ = repo.Set(ctx, "server.language", "es")
	if d := LocaleDefaults(ctx, repo, "fr"); d["metadata.language"] != "es" || d["server.language"] != "fr" {
		t.Errorf("server language set: %v, want metadata to follow it", d)
	}
	t.Setenv("TZ", "")
	if d := LocaleDefaults(ctx, repo, "fr"); d[KeyServerTimezone] != DefaultServerTimezone {
		t.Errorf("no TZ: zone %q, want %s", d[KeyServerTimezone], DefaultServerTimezone)
	}
}
