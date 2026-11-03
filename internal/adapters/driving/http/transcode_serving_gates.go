package httpapi

// transcode_serving_gates.go - the packaging switches an admin toggles while
// the server runs (transcoding.remux_copy, transcoding.hls_ondemand), applied
// to the NEXT playback and never to one already running.
//
// Every child request of a playback (variant, segment, init, audio rendition)
// re-resolves the plan from its URL. Were it to read the live setting, a
// toggle would move a running session onto another rung: another
// fingerprint, and every URL its player holds refused 410. So the setting is
// read where a playback starts (the master, the DASH manifest, a prewarm),
// and a closed gate is stamped into the request (rc=0 / pk=0) that the
// manifest bakes into every child URI, like the vd= pin. A child URL without
// the stamp was minted while the gate was open, and stays open.

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

const (
	// remuxCopyGateParam = "0" closes the ffmpeg copy rung for the playback.
	remuxCopyGateParam = "rc"
	// packagerGateParam = "0" closes the on-demand packager for the playback.
	packagerGateParam = "pk"

	// servingFlagRefresh bounds how often a master reads a switch from the
	// config store: a burst of masters costs one read, and a toggle reaches
	// new playbacks within this delay.
	servingFlagRefresh = 5 * time.Second
	// servingFlagReadTimeout bounds how long a master waits on the store.
	servingFlagReadTimeout = 500 * time.Millisecond
)

// liveServerFlag is an operator flag read at use time (row, then env, then
// default), the last value kept for refresh. It sits on the master path, so
// the store is never trusted to answer: one read is in flight at a time, its
// caller waits at most readTimeout, every other caller answers with the last
// value read, and a failed read keeps that value - a switch the admin turned
// off does not reopen on a transient store error. The default stands only
// until a first read succeeds. A nil flag is open: a handler built without
// one keeps its wiring as the only gate.
type liveServerFlag struct {
	cfgRepo     shared.ServerConfigRepository
	dbKey       string
	envVar      string
	defaultOn   bool
	refresh     time.Duration
	readTimeout time.Duration
	now         func() time.Time

	mu       sync.Mutex
	value    bool
	readAt   time.Time
	tried    bool
	inflight chan struct{} // closed when the read in flight lands; nil when none
}

func newLiveServerFlag(cfgRepo shared.ServerConfigRepository, dbKey, envVar string, defaultOn bool) *liveServerFlag {
	return &liveServerFlag{
		cfgRepo:     cfgRepo,
		dbKey:       dbKey,
		envVar:      envVar,
		defaultOn:   defaultOn,
		refresh:     servingFlagRefresh,
		readTimeout: servingFlagReadTimeout,
		now:         time.Now,
		value:       flagEnabled(os.Getenv(envVar), defaultOn),
	}
}

func (f *liveServerFlag) enabled() bool {
	if f == nil {
		return true
	}
	f.mu.Lock()
	if f.inflight != nil || (f.tried && f.now().Sub(f.readAt) < f.refresh) {
		v := f.value
		f.mu.Unlock()
		return v
	}
	done := make(chan struct{})
	f.inflight = done
	f.mu.Unlock()

	// Detached from any request on purpose: the value is shared by every
	// master, and one viewer leaving must not cancel the read the others
	// will use. The timeout bounds the store, the goroutine outlives a store
	// that ignores it, and the in-flight mark keeps that to one.
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), f.readTimeout)
		defer cancel()
		raw, err := readServerFlag(ctx, f.cfgRepo, f.dbKey, f.envVar)
		f.mu.Lock()
		if err == nil {
			f.value = flagEnabled(raw, f.defaultOn)
		} else {
			slog.Warn("server flag: read failed, keeping the last value",
				slog.String("key", f.dbKey), slog.Bool("value", f.value), slog.Any("err", err))
		}
		f.readAt, f.tried, f.inflight = f.now(), true, nil
		f.mu.Unlock()
		close(done)
	}()

	timer := time.NewTimer(f.readTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.value
}

// readServerFlag is resolveServerFlag that reports a store failure instead
// of reading it as "unset": a missing row is unset, an error is an error.
func readServerFlag(ctx context.Context, cfgRepo shared.ServerConfigRepository, dbKey, envVar string) (string, error) {
	if cfgRepo != nil {
		for _, k := range [...]string{dbKey, envVar} {
			entry, err := cfgRepo.Get(ctx, k)
			switch {
			case err == nil && entry != nil && entry.Value != "":
				return entry.Value, nil
			case err != nil && !errors.Is(err, shared.ErrNotFound):
				return "", err
			}
		}
	}
	return os.Getenv(envVar), nil
}

// stampServingGates is called where a playback starts: each switch that is
// off now is stamped closed on the request, so the plan resolved from it and
// every URI its manifest bakes carry that decision. A switch that is on
// stamps nothing (a client asking for less on its own is honoured).
func (h *hlsVODServe) stampServingGates(r *http.Request) *http.Request {
	q := r.URL.Query()
	changed := false
	for _, g := range [...]struct {
		param string
		flag  *liveServerFlag
	}{
		{remuxCopyGateParam, h.remuxCopyFlag},
		{packagerGateParam, h.packagerFlag},
	} {
		if !g.flag.enabled() && q.Get(g.param) != "0" {
			q.Set(g.param, "0")
			changed = true
		}
	}
	if !changed {
		return r
	}
	stamped := r.Clone(r.Context())
	stamped.URL.RawQuery = q.Encode()
	return stamped
}

// servingGates says which packaging engines a request may use: wired, and
// not closed by the playback's stamp. A nil request (the pre-play info
// surfaces) answers with the current settings, as the next master would.
func (h *hlsVODServe) servingGates(r *http.Request) (copyOpen, packagerOpen bool) {
	copyOpen, packagerOpen = h.copyProducer != nil, h.onDemand != nil
	if r == nil {
		return copyOpen && h.remuxCopyFlag.enabled(), packagerOpen && h.packagerFlag.enabled()
	}
	q := r.URL.Query()
	return copyOpen && q.Get(remuxCopyGateParam) != "0", packagerOpen && q.Get(packagerGateParam) != "0"
}

// gateClosed reports whether the request carries a closed stamp for param:
// what the manifest bakes into its child URIs.
func gateClosed(r *http.Request, param string) bool {
	return r != nil && r.URL.Query().Get(param) == "0"
}
