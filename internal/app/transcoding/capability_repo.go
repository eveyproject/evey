package transcoding

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// CapabilityConfigKey is the server_settings row that stores the JSON-
// encoded EncodeCapability. Reserved namespace under `transcoding.` so
// it can't collide with other config knobs.
const CapabilityConfigKey = "transcoding.encode_capability"

// CapabilityRepoAdapter implements domain CapabilityRepo on top of the
// generic ServerConfigRepository - no schema migration needed, we
// piggyback on the existing server_settings table.
//
// JSON encoding lets the EncodeCapability struct evolve (new fields,
// renames) without touching the storage layer; the adapter only sees
// the byte string.
type CapabilityRepoAdapter struct {
	store domainshared.ServerConfigRepository
}

func NewCapabilityRepoAdapter(store domainshared.ServerConfigRepository) *CapabilityRepoAdapter {
	return &CapabilityRepoAdapter{store: store}
}

// Get returns the persisted capability + a bool indicating whether
// anything was actually loaded. (found=false, err=nil) means "no
// calibration yet on this host" - callers should treat it as a signal
// to run the BenchmarkRunner.
func (r *CapabilityRepoAdapter) Get(ctx context.Context) (domaintranscoding.EncodeCapability, bool, error) {
	entry, err := r.store.Get(ctx, CapabilityConfigKey)
	if err != nil {
		// ErrNotFound is the "no row" signal - distinguish from real DB
		// failures so first-boot doesn't log an ERROR every time. Treat
		// missing key as found=false (run the benchmark).
		if errors.Is(err, domainshared.ErrNotFound) {
			return domaintranscoding.EncodeCapability{}, false, nil
		}
		return domaintranscoding.EncodeCapability{}, false, fmt.Errorf("capability repo: get: %w", err)
	}
	if entry == nil || entry.Value == "" {
		return domaintranscoding.EncodeCapability{}, false, nil
	}
	var cap domaintranscoding.EncodeCapability
	if err := json.Unmarshal([]byte(entry.Value), &cap); err != nil {
		// Corrupt JSON in storage is treated as "no calibration": the
		// caller will re-run the benchmark and overwrite the bad row
		// instead of failing the boot.
		return domaintranscoding.EncodeCapability{}, false, nil //nolint:nilerr // intentional: corrupt cache → trigger rebench, not propagate the parse error
	}
	return cap, true, nil
}

// Set persists the capability as JSON. The serialization itself should
// never fail (the struct is plain data) but we wrap the error for
// observability anyway - admins look at this kind of log when
// recalibration silently fails.
func (r *CapabilityRepoAdapter) Set(ctx context.Context, cap domaintranscoding.EncodeCapability) error {
	if cap.SoftwareMaxHeight <= 0 {
		// Refuse to persist a useless calibration - the safe fallback
		// (720p) is implicit when Get returns found=false. Persisting
		// zero would lock the host into "uncalibrated" state forever.
		return errors.New("capability repo: refuse to persist zero SoftwareMaxHeight")
	}
	// Upper-bound guard - defends against a corrupted JSON write (DB
	// access by an external tool, future config-import bug, etc.)
	// that would push a height beyond any sensible video resolution.
	// CapLadderToHeight tolerates large values gracefully (always
	// returns >=1 rung) so this is defense-in-depth rather than a
	// hot-path check, but the cap aligns with the highest realistic
	// rung any benchmarker could produce.
	if cap.SoftwareMaxHeight > 4320 || cap.HWMaxHeight > 4320 {
		return errors.New("capability repo: refuse to persist height > 4320 (above 8K - likely corrupted)")
	}
	b, err := json.Marshal(cap)
	if err != nil {
		return fmt.Errorf("capability repo: marshal: %w", err)
	}
	if err := r.store.Set(ctx, CapabilityConfigKey, string(b)); err != nil {
		return fmt.Errorf("capability repo: set: %w", err)
	}
	return nil
}
