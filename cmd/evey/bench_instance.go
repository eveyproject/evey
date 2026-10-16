package main

import (
	"log/slog"

	"github.com/eveyproject/evey/internal/config"
)

// benchInstance is EVEY_BENCH_INSTANCE, set once at boot before anything is
// wired: a server run as a load-test bench beside a production one starts no
// background work at all (see config.ServerConfig.BenchInstance). Read by
// the wire-up helpers that do not carry the config.
var benchInstance bool

// benchInstanceOff lists what a bench instance does not start, for its boot
// warning and banner.
const benchInstanceOff = "library watcher and scans, re-probes, trickplay, intro detection, calibration bench, keyframe/level/complexity backfills, title probes, metadata enrichment, search reindex, backups, notifications, plugins, update check, metrics sampling"

// announceBenchInstance logs, once and loudly, that this server is a bench.
func announceBenchInstance(cfg *config.Config, log *slog.Logger) {
	benchInstance = cfg.Server.BenchInstance
	if !benchInstance {
		return
	}
	log.Warn("BENCH INSTANCE: every background task is off; this server only serves requests and load tests",
		slog.String("off", benchInstanceOff))
}
