package main

import (
	"context"
	"log/slog"
	"runtime/debug"
	"time"

	meili "github.com/eveyproject/evey/internal/adapters/driven/meilisearch"
	"github.com/eveyproject/evey/internal/adapters/driven/search"
	applibrary "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// wire_events.go starts the background workers, the filesystem watcher and the
// event-bus subscriptions once the app services are built.

// buildEventWiring starts background workers, filesystem watcher, and event bus subscriptions.
func buildEventWiring(ctx context.Context, cfg *config.Config, infra *infraDeps, deps *appDeps, rootLog *slog.Logger) {
	r := infra.repos
	bus := infra.bus

	// Apply any admin DB overrides for the library watcher + periodic scan
	// BEFORE they start (these settings are consumed here, not re-read later, so
	// a change takes effect on the next restart - see /admin/server-settings).
	loadOverride(ctx, deps.cfgStore, "library.watcher_enabled", parseBool, &cfg.Library.WatcherEnabled)
	loadOverride(ctx, deps.cfgStore, "library.periodic_scan_interval", parseDuration, &cfg.Library.PeriodicScanInterval)

	watcherSvc := startWatcher(ctx, cfg, bus, r.Library, deps.library.cmdSvc, rootLog)
	logLibrarySourceAccess(ctx, r.Library, rootLog)

	rootLog.Info("periodic scan started for all libraries", slog.Duration("interval", cfg.Library.PeriodicScanInterval))
	// The periodic walk is now a BACKSTOP, not the primary discovery path:
	// libraries whose watch answers the liveness probe are only walked every
	// watchedScanInterval, the rest keep this configured cadence. A startup pass
	// runs regardless - inotify is blind to whatever happened while we were down.
	if watcherSvc != nil {
		deps.library.cmdSvc.WithWatchHealth(watcherSvc.AliveWatchedLibraries)
	}
	deps.library.cmdSvc.StartPeriodicScan(ctx, r.Library, cfg.Library.PeriodicScanInterval, nil)

	// One-shot async backfill of media_items.sort_title for rows scanned
	// before the column was populated. Idempotent - exits cheaply once
	// every row has a non-empty value. Async so we never block boot on
	// it; the only consumer (the lib listing keyset cursor) tolerates
	// a brief window of UUID-sorted results.
	go func() {
		defer func() {
			if rec := recover(); rec != nil {
				rootLog.Error("sort_title backfill panic", slog.Any("error", rec))
			}
		}()
		n, err := r.MediaItem.BackfillEmptySortTitles(ctx)
		if err != nil {
			rootLog.Warn("sort_title backfill", slog.Any("err", err))
			return
		}
		if n > 0 {
			rootLog.Info("sort_title backfill complete", slog.Int("rows_updated", n))
		}
	}()
	if infra.tmdbKey != "" {
		deps.content.metadataCmdSvc.StartEnrichmentWorker(ctx)
	}
	// Periodic reconciliation of artwork records that have a SourceURL but
	// no local file on disk - heals transient CDN/network failures from
	// the initial enrichment burst without admin intervention. Runs
	// regardless of provider keys: catches up records inserted by any
	// metadata path (TMDB, OMDb, manual override).
	go deps.content.metadataCmdSvc.RetryMissingArtworksLoop(ctx, time.Hour)

	// Live-reload the artwork auto-download flag on server-config change.
	// Without this the admin toggle would only take effect on next server
	// restart - defeats the point of having a UI switch.
	bus.Subscribe(shared.ServerConfigChanged{}.EventName(), func(_ context.Context, e shared.Event) {
		ev, ok := e.(shared.ServerConfigChanged)
		if !ok || ev.Key != "artwork.auto_download" {
			return
		}
		// Deletion / empty value falls back to the default (true), matching
		// resolveAutoDownloadDefault's semantics on cold start.
		enabled := ev.Deleted || ev.Value != "false"
		deps.content.metadataCmdSvc.SetAutoDownload("/data", enabled)
		slog.Info("artwork auto-download toggled via config", slog.Bool("enabled", enabled))
	})
	deps.library.cmdSvc.StartTrickplayWorker(ctx)
	deps.library.cmdSvc.StartBackgroundGeneration(ctx)
	if deps.library.introDetectionSvc != nil {
		deps.library.introDetectionSvc.WithEventBus(bus)
		deps.library.introDetectionSvc.StartWorker(ctx)
	}

	if rs, ok := infra.searchAdapter.(*search.ResilientSearch); ok {
		if sa, isMeili := rs.Primary().(*meili.SearchAdapter); isMeili {
			// Reconcile the media search index against the DB. The resilient wrapper
			// swallows writes while Meili is down, so an item created/edited during an
			// outage stays out of the index. ReconcileSearchIndex rebuilds when the
			// engine's doc count diverges from the DB count. Pass `sa` (the PRIMARY,
			// not the swallowing wrapper) so a still-down engine surfaces its error
			// and the drift isn't falsely cleared. Run at boot AND on a ticker so a
			// mid-session outage self-heals without a restart.
			reconcileSearch := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(ctx, "panic recovered in search reconcile", "error", r, "stack", string(debug.Stack()))
					}
				}()
				rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				if _, err := applibrary.ReconcileSearchIndex(rctx, infra.repos.MediaItemQuery, sa, sa, infra.repos.MediaItemTranslation, infra.repos.MediaPerson); err != nil {
					slog.WarnContext(rctx, "search reconcile failed", slog.Any("err", err))
				}
			}
			// Boot: full upsert reindex FIRST, then reconcile. The count-based
			// reconcile is blind to drift that nets to zero (adds and deletes
			// balancing out during outages left ~half the prod series out of the
			// index on 2026-08-07 while both counts matched). The upsert rebuild
			// restores every missing/stale-valued doc without the empty-search
			// window a wipe would cause; if extra stale docs remain, the index is
			// then LARGER than the DB, which the reconcile right after finally
			// sees and heals with its wipe+rebuild. Tickers stay count-based -
			// the full sweep is a boot-only cost.
			go func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(ctx, "panic recovered in boot search reindex", "error", r, "stack", string(debug.Stack()))
					}
				}()
				rctx, cancel := context.WithTimeout(ctx, 30*time.Minute)
				defer cancel()
				n, err := applibrary.ReindexSearchIndex(rctx, infra.repos.MediaItemQuery, sa, infra.repos.MediaItemTranslation, infra.repos.MediaPerson, "", nil)
				if err != nil {
					slog.WarnContext(rctx, "boot search reindex failed (engine down? reconcile ticker will retry the count path)", slog.Any("err", err))
				} else {
					slog.InfoContext(rctx, "boot search reindex complete", slog.Int("indexed", n))
				}
				reconcileSearch()
			}()
			go func() {
				ticker := time.NewTicker(15 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						reconcileSearch()
					}
				}
			}()

			// Drain the durable search write-queue: replay writes the engine
			// rejected while it was down (recorded by the resilient wrapper into the
			// pending_search_index outbox). This is the PRIMARY drift-prevention path
			// - no write is lost - so the count-based reconcile above is left only for
			// the total-index-loss case it handles well. Runs often and cheap (the
			// queue is normally empty), so a recovered engine catches up within a
			// minute. Uses `sa` (the PRIMARY) so a still-down engine surfaces its error
			// and the ops stay queued instead of being dropped.
			drainSearch := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(ctx, "panic recovered in search write-queue drain", "error", r, "stack", string(debug.Stack()))
					}
				}()
				dctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
				defer cancel()
				if _, err := applibrary.DrainSearchWriteQueue(dctx, infra.repos.PendingSearchWrite, infra.repos.MediaItemQuery, sa, infra.repos.MediaItemTranslation, infra.repos.MediaPerson); err != nil {
					slog.WarnContext(dctx, "search write-queue drain failed (engine still down?)", slog.Any("err", err))
				}
			}
			go drainSearch()
			go func() {
				ticker := time.NewTicker(1 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						drainSearch()
					}
				}
			}()
		}
	}

	// Reconcile the music search index against the DB - same drift as the media
	// index: the resilient wrapper swallows writes while Meili is down, so a track
	// added during an outage stays unsearchable. ReconcileTracks rebuilds when the
	// engine's doc count diverges from the DB track count (this also covers the
	// first-boot empty-index backfill: 0 docs vs N tracks is a divergence). Targets
	// the PRIMARY (msa) so a still-down engine surfaces its error. Boot + ticker so
	// a mid-session outage self-heals without a restart.
	if rm, ok := infra.musicSearch.(*search.ResilientMusicSearch); ok {
		if msa, isMeili := rm.Primary().(*meili.MusicSearchAdapter); isMeili {
			reconcileMusic := func() {
				defer func() {
					if r := recover(); r != nil {
						slog.ErrorContext(ctx, "panic recovered in music search reconcile", "error", r, "stack", string(debug.Stack()))
					}
				}()
				rctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
				defer cancel()
				if _, err := deps.media.musicSvc.CommandService().ReconcileTracks(rctx, msa, msa); err != nil {
					slog.WarnContext(rctx, "music search reconcile failed", slog.Any("err", err))
				}
			}
			go reconcileMusic()
			go func() {
				ticker := time.NewTicker(15 * time.Minute)
				defer ticker.Stop()
				for {
					select {
					case <-ctx.Done():
						return
					case <-ticker.C:
						reconcileMusic()
					}
				}
			}()
		}
	}
}
