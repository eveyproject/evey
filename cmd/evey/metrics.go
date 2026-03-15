package main

import (
	"context"
	"database/sql"
	"time"

	apptranscoding "github.com/gravity-zero/evey/internal/app/transcoding"
	domainlibrary "github.com/gravity-zero/evey/internal/domain/library"
	eveymetrics "github.com/gravity-zero/evey/internal/pkg/metrics"
)

type mediaItemCounter interface {
	Count(ctx context.Context, filter domainlibrary.ListMediaItemsFilter) (int, error)
}

func startMetricsCollector(ctx context.Context, db *sql.DB, hlsManager *apptranscoding.HLSManager, mediaItemRepo mediaItemCounter) {
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			eveymetrics.HLSSessionsGauge.Set(float64(hlsManager.ActiveSessionCount()))
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
		}
	}()

	go func() {
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
