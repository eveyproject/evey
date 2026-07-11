package main

import (
	"context"
	"database/sql"
	"log/slog"
	"runtime/debug"
	"time"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	eveymetrics "github.com/evey-project/evey/internal/pkg/metrics"
)

type mediaItemCounter interface {
	Count(ctx context.Context, filter domainlibrary.ListMediaItemsFilter) (int, error)
}

func startMetricsCollector(ctx context.Context, db *sql.DB, hlsOrch *apptranscoding.SegmentOrchestrator, mediaItemRepo mediaItemCounter) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in metrics HLS collector", "error", r, "stack", string(debug.Stack()))
			}
		}()
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			// Keep the legacy gauge name so existing Prometheus dashboards
			// keep working - the VOD orchestrator's per-event metrics
			// (evey_hlsvod_*) sit alongside.
			eveymetrics.HLSSessionsGauge.Set(float64(hlsOrch.ActiveSessionCount()))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic recovered in metrics DB collector", "error", r, "stack", string(debug.Stack()))
			}
		}()
		dbTicker := time.NewTicker(30 * time.Second)
		mediaTicker := time.NewTicker(5 * time.Minute)
		defer dbTicker.Stop()
		defer mediaTicker.Stop()

		updateDB := func() {
			s := db.Stats()
			eveymetrics.DBConnectionsOpen.Set(float64(s.OpenConnections))
			eveymetrics.DBConnectionsIdle.Set(float64(s.Idle))
			eveymetrics.DBConnectionsInUse.Set(float64(s.InUse))
		}
		updateMedia := func() {
			for _, t := range []string{"movie", "series", "episode"} {
				typ := t
				n, err := mediaItemRepo.Count(ctx, domainlibrary.ListMediaItemsFilter{Type: &typ})
				if err == nil {
					eveymetrics.MediaItemsTotal.WithLabelValues(t).Set(float64(n))
				}
			}
		}

		updateDB()
		updateMedia()
		for {
			select {
			case <-ctx.Done():
				return
			case <-dbTicker.C:
				updateDB()
			case <-mediaTicker.C:
				updateMedia()
			}
		}
	}()
}
