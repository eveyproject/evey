//go:build boottest

package main

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/evey-project/evey/internal/adapters/driven/postgres/testhelpers"
	"github.com/evey-project/evey/internal/config"
)

// TestBuildServices_Boot is the composition-root smoke test: it wires the FULL
// application (buildServices -> buildInfra + buildAppServices + buildEventWiring)
// against a real migrated Postgres and asserts every critical service came out
// non-nil. It is the safety net a future decomposition of buildAppServices needs:
// a helper that forgets to assign a field, or returns nil, trips here even though
// the code still compiles and boots to "listening" (a nil service only panics
// when first USED, which a boot-to-listening check does not exercise).
//
// Build-tagged `boottest` so it never runs in the default pre-push gate - it is
// heavy (real container + full boot, incl. the ~20s calibration bench when an
// ffmpeg is present). Run it explicitly before touching wire.go:
//
//	make boot-itest
func TestBuildServices_Boot(t *testing.T) {
	testhelpers.SkipIfNoDocker(t)
	if testing.Short() {
		t.Skip("skipping boot integration test under -short")
	}

	db, cleanup := testhelpers.StartPostgres(t)
	defer cleanup()

	// A self-contained config: temp dirs for everything the boot writes/reads,
	// no external search (empty URL -> SQL fallback), no sandbox tooling, and an
	// OpenSubtitles key so the (otherwise optional) subtitle service is actually
	// built - proving that conditional wiring too. The JWT/HLS keys are generated
	// on the fly under tmp by LoadOrGenerate*.
	tmp := t.TempDir()
	t.Setenv("EVEY_MEILI_URL", "")
	t.Setenv("EVEY_TRANSCODING_CACHE_DIR", filepath.Join(tmp, "transcode"))
	t.Setenv("EVEY_TRICKPLAY_CACHE_DIR", filepath.Join(tmp, "trickplay"))
	t.Setenv("EVEY_SEEKINDEX_STATE_DIR", filepath.Join(tmp, "seek-index"))
	t.Setenv("EVEY_BACKUP_DIR", filepath.Join(tmp, "backups"))
	t.Setenv("EVEY_LOG_DIR", filepath.Join(tmp, "logs"))
	t.Setenv("EVEY_JWT_PRIVATE_KEY_PATH", filepath.Join(tmp, "jwt.key"))
	t.Setenv("EVEY_PLUGINS_SANDBOX_MODE", "none")
	t.Setenv("EVEY_OPENSUBTITLES_API_KEY", "boot-test-key")

	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config.Load: %v", err)
	}

	// Cancel tears down every background goroutine buildServices launched
	// (workers, watcher, plugin idle manager, cache eviction) once we're done.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	deps, err := buildServices(ctx, cfg, db, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("buildServices returned error: %v", err)
	}
	if deps == nil {
		t.Fatal("buildServices returned nil deps")
	}

	// Every service a running server depends on must be wired. This list IS the
	// guard: a decomposition that drops one trips here. It mirrors appDeps and
	// covers all six extracted builders (notification / feed / music / playlist /
	// subtitle / backup) plus the core services.
	checks := []struct {
		name  string
		isNil bool
	}{
		{"library.cmdSvc", deps.library.cmdSvc == nil},
		{"library.querySvc", deps.library.querySvc == nil},
		{"library.scheduler", deps.library.scheduler == nil},
		{"user.authSvc", deps.user.authSvc == nil},
		{"user.userSvc", deps.user.userSvc == nil},
		{"user.accessSvc", deps.user.accessSvc == nil},
		{"user.querySvc", deps.user.querySvc == nil},
		{"media.playbackSvc", deps.media.playbackSvc == nil},
		{"media.transcodingSvc", deps.media.transcodingSvc == nil},
		{"media.musicSvc", deps.media.musicSvc == nil},
		{"media.musicNowPlaying", deps.media.musicNowPlaying == nil},
		{"media.subtitleSvc", deps.media.subtitleSvc == nil},
		{"media.syncPlayHub", deps.media.syncPlayHub == nil},
		{"media.shareSvc", deps.media.shareSvc == nil},
		{"content.metadataCmdSvc", deps.content.metadataCmdSvc == nil},
		{"content.metadataQuerySvc", deps.content.metadataQuerySvc == nil},
		{"content.collectionCmdSvc", deps.content.collectionCmdSvc == nil},
		{"content.collectionQuerySvc", deps.content.collectionQuerySvc == nil},
		{"content.favoriteSvc", deps.content.favoriteSvc == nil},
		{"content.mediaRequestSvc", deps.content.mediaRequestSvc == nil},
		{"repos.userRatingRepo", deps.repos.userRatingRepo == nil},
		{"content.playlistSvc", deps.content.playlistSvc == nil},
		{"content.playlistGenSvc", deps.content.playlistGenSvc == nil},
		{"engagement.notifCmdSvc", deps.engagement.notifCmdSvc == nil},
		{"engagement.notifQuerySvc", deps.engagement.notifQuerySvc == nil},
		{"engagement.feedCmdSvc", deps.engagement.feedCmdSvc == nil},
		{"engagement.feedQuerySvc", deps.engagement.feedQuerySvc == nil},
		{"engagement.webhookSvc", deps.engagement.webhookSvc == nil},
		{"engagement.pluginInstallerSvc", deps.engagement.pluginInstallerSvc == nil},
		{"engagement.pluginRuntimeSvc", deps.engagement.pluginRuntimeSvc == nil},
		{"auth.tokenGen", deps.auth.tokenGen == nil},
		{"backupSvc", deps.backupSvc == nil},
		{"backupScheduler", deps.backupScheduler == nil},
		{"bus", deps.bus == nil},
		{"serverCfgSvc", deps.serverCfgSvc == nil},
		{"pluginRunner", deps.pluginRunner == nil},
	}
	for _, c := range checks {
		if c.isNil {
			t.Errorf("appDeps: %s is nil after buildServices - a required service was not wired", c.name)
		}
	}

	// The transcode background workers must register their blocking stoppers so
	// shutdown drains them before db.Close. Default-on wiring starts four
	// (cap-override, complexity, level-backfill, keyframe-backfill); a worker that
	// starts a goroutine but forgets to register its stopper leaves an in-flight
	// write racing the closing pool - invisible to the nil checks above.
	if len(deps.bgStoppers) == 0 {
		t.Error("appDeps.bgStoppers is empty - no transcode worker registered a shutdown drain")
	}
}
