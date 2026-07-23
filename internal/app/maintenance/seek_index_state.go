// Seek-index state persistence. Two small artifacts live in the state dir:
//
//   - scans.json: the per-scope cached scan results. A library scan is minutes
//     of per-file probes (head parse + ffprobe); losing it to a restart forced
//     the operator to re-walk everything before repairing. Persisted on every
//     scan/job completion, loaded at construction.
//   - job.json: a marker for the currently running job, refreshed as files
//     complete. Jobs are in-memory goroutines - a deploy kills them silently.
//     At boot a leftover marker becomes the "last job" summary with an
//     interrupted error, so the operator sees the truth instead of assuming
//     the batch finished.
//
// Both are best-effort: persistence failures are logged and never block the
// scan/repair itself.
package maintenance

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"log/slog"
)

const (
	stateScansFile = "scans.json"
	stateJobFile   = "job.json"
)

// WithStateDir enables state persistence under dir (created if needed) and
// immediately restores any persisted scans and interrupted-job marker.
// Optional and chainable; without it the service is memory-only (historic
// behaviour).
func (s *SeekIndexService) WithStateDir(dir string) *SeekIndexService {
	if dir == "" {
		return s
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		s.logger.Warn("seek-index: state dir unavailable, persistence disabled",
			slog.String("dir", dir), slog.Any("err", err))
		return s
	}
	s.stateDir = dir
	s.loadScans()
	s.consumeInterruptedJob()
	return s
}

// persistedScans is the on-disk shape of the scan cache.
type persistedScans struct {
	Scopes map[string]persistedScan `json:"scopes"`
}

type persistedScan struct {
	At    time.Time    `json:"at"`
	Items []SeekStatus `json:"items"`
}

// saveScans snapshots the whole cache. Called with s.mu NOT held.
func (s *SeekIndexService) saveScans() {
	if s.stateDir == "" {
		return
	}
	s.mu.Lock()
	out := persistedScans{Scopes: make(map[string]persistedScan, len(s.scans))}
	for k, c := range s.scans {
		out.Scopes[k] = persistedScan{At: c.at, Items: append([]SeekStatus(nil), c.res.Items...)}
	}
	s.mu.Unlock()

	data, err := json.Marshal(out)
	if err != nil {
		s.logger.Warn("seek-index: marshal scan cache failed", slog.Any("err", err))
		return
	}
	tmp := filepath.Join(s.stateDir, stateScansFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		s.logger.Warn("seek-index: persist scan cache failed", slog.Any("err", err))
		return
	}
	if err := os.Rename(tmp, filepath.Join(s.stateDir, stateScansFile)); err != nil {
		s.logger.Warn("seek-index: swap scan cache failed", slog.Any("err", err))
	}
}

// loadScans restores the cache written by a previous process. Rollups are
// recomputed from the items - the file carries only facts.
func (s *SeekIndexService) loadScans() {
	data, err := os.ReadFile(filepath.Join(s.stateDir, stateScansFile))
	if err != nil {
		return // no persisted cache - fine
	}
	var in persistedScans
	if err := json.Unmarshal(data, &in); err != nil {
		s.logger.Warn("seek-index: persisted scan cache unreadable, ignoring", slog.Any("err", err))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for k, p := range in.Scopes {
		var res ScanResult
		res.Items = p.Items
		res.Total = len(p.Items)
		for _, it := range p.Items {
			if !it.HasIndex {
				res.MissingIndex++
			}
			if it.HasBackup {
				res.WithBackup++
			}
			if it.AudioDelaySec > AudioDelayFlagSec {
				res.AudioDelayed++
			}
		}
		s.scans[k] = cachedScan{res: res, at: p.At}
	}
}

// persistedJob is the running-job marker.
type persistedJob struct {
	Kind      string    `json:"kind"`
	Scope     string    `json:"scope"`
	Phase     string    `json:"phase"`
	Done      int       `json:"done"`
	Failed    int       `json:"failed"`
	Total     int       `json:"total"`
	StartedAt time.Time `json:"startedAt"`
}

// marshalJobMarker snapshots the running job into its persisted bytes. Called
// with s.mu held (it reads s.running + s.cur); returns nil when there is nothing
// to persist (no state dir, or no job running) so the paired write no-ops.
func (s *SeekIndexService) marshalJobMarker() []byte {
	if s.stateDir == "" || !s.running {
		return nil
	}
	data, err := json.Marshal(persistedJob{
		Kind: s.cur.Kind, Scope: s.cur.Scope, Phase: s.cur.Phase,
		Done: s.cur.Done, Failed: s.cur.Failed, Total: s.cur.Total,
		StartedAt: s.cur.StartedAt,
	})
	if err != nil {
		return nil
	}
	return data
}

// writeJobMarker persists the bytes marshalJobMarker produced. Called WITHOUT
// s.mu held so a slow NAS disk write never blocks Status()/Cancel() mid-batch
// (the marshal already happened under the lock). Jobs are serialized on one
// goroutine, so there is no concurrent writer of this file.
func (s *SeekIndexService) writeJobMarker(data []byte) {
	if data == nil {
		return
	}
	if err := os.WriteFile(filepath.Join(s.stateDir, stateJobFile), data, 0o644); err != nil {
		s.logger.Warn("seek-index: persist job marker failed", slog.Any("err", err))
	}
}

// clearJobMarker removes the marker once the job finished cleanly. Best-effort.
func (s *SeekIndexService) clearJobMarker() {
	if s.stateDir == "" {
		return
	}
	if err := os.Remove(filepath.Join(s.stateDir, stateJobFile)); err != nil && !os.IsNotExist(err) {
		s.logger.Warn("seek-index: clear job marker failed", slog.Any("err", err))
	}
}

// consumeInterruptedJob turns a leftover marker into the last-job summary so
// the UI reports "interrupted at X/Y" instead of pretending nothing ran. The
// marker is deleted either way (one-shot).
func (s *SeekIndexService) consumeInterruptedJob() {
	path := filepath.Join(s.stateDir, stateJobFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	_ = os.Remove(path) //nolint:errcheck // one-shot marker: a failed delete only re-reports the interruption once more
	var j persistedJob
	if err := json.Unmarshal(data, &j); err != nil {
		s.logger.Warn("seek-index: job marker unreadable, dropped", slog.Any("err", err))
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.last = &JobSummary{
		Kind: j.Kind, Scope: j.Scope,
		Done: j.Done, Failed: j.Failed, Total: j.Total,
		FinishedAt: time.Now(),
		Err:        "interrupted by a server restart - relaunch it to finish the remaining files",
	}
	s.logger.Warn("seek-index: previous job was interrupted by a restart",
		slog.String("kind", j.Kind), slog.String("scope", j.Scope),
		slog.Int("done", j.Done), slog.Int("total", j.Total))
}
