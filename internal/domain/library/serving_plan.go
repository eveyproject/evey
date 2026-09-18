package library

import "context"

// ServingStrategy is Evey's domain vocabulary for a file's playback delivery
// decision, mirroring mkvgo's ops.ServingStrategy one-for-one (translated at
// the adapter boundary so the mkvgo type never leaks into this package).
type ServingStrategy string

const (
	ServingStrategyDirectPlay ServingStrategy = "direct-play"
	ServingStrategyRemuxHLS   ServingStrategy = "remux-hls"
	ServingStrategyTranscode  ServingStrategy = "transcode"
)

// ServingPlan is mkvgo's ops.Ingest verdict on one source file, computed at
// scan time by IngestPlanner and consumed AT SCAN TIME only: NeedsReindex
// drives the inline seek-index repair (mode plan+repair) and Strategy the
// keyframe warm-up that makes a fresh file copy-eligible before its first
// play. It is never persisted and the playback decision never reads it -
// the plan resolver decides from the probed streams and the client.
type ServingPlan struct {
	// Target is the client target the plan was judged for ("mse-generic" by default).
	Target string
	// Strategy is mkvgo's delivery verdict for that target.
	Strategy ServingStrategy
	// NeedsReindex: the source lacks a usable seek index (Cues); mkvgo can
	// rebuild it in place.
	NeedsReindex bool
}

// IngestPlanner computes a ServingPlan for a source path (mkvgo ops.Ingest).
type IngestPlanner interface {
	Plan(ctx context.Context, path, target string) (*ServingPlan, error)
}
