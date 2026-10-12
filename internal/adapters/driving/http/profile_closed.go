package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"

	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/metrics"
)

// ProfileAccessGate says whether a profile may be used now (kids opening
// hours). Implemented by app/user.ProfileSchedules.
type ProfileAccessGate interface {
	Access(ctx context.Context, profileID string) (domainuser.ProfileAccess, error)
}

type gateHolder struct{ g ProfileAccessGate }

// profileGate is set once at wiring. Every authenticated HTTP route goes
// through httpAuthMiddleware or httpStreamAuthMiddleware, so checking there
// closes everything a kids profile can reach outside its hours - browsing,
// every HLS segment, direct play, downloads, music - in one place.
var profileGate atomic.Pointer[gateHolder]

// SetProfileAccessGate installs the gate (nil removes it).
func SetProfileAccessGate(g ProfileAccessGate) {
	if g == nil {
		profileGate.Store(nil)
		return
	}
	profileGate.Store(&gateHolder{g: g})
}

// ProfileClosedReason is the machine-readable reason clients match on, in the
// same ErrorInfo shape the gRPC gateway serves.
const ProfileClosedReason = "PROFILE_CLOSED"

// allowedWhileClosed are the routes a closed kids profile still needs: read
// its hours and lift them with a parent's PIN, verify a PIN and show the
// avatars to switch to another profile. Everything else answers
// profile_closed.
func allowedWhileClosed(r *http.Request) bool {
	p := r.URL.Path
	if !strings.HasPrefix(p, "/api/v1/profiles/") {
		return false
	}
	get := r.Method == http.MethodGet
	return strings.HasSuffix(p, "/schedule") && get ||
		strings.HasSuffix(p, "/schedule/unlock") ||
		strings.HasSuffix(p, "/parental/verify-pin") ||
		// The profile picker a closed profile is sent to shows every face.
		strings.HasSuffix(p, "/avatar") && get
}

// gateProfile applies a kids profile's opening hours to one request. Outside
// them it answers 403 profile_closed (refused = true). Inside them, when the
// opening ends while the response is still being written - a film read as one
// long range, a download, a music track - the response is cut at the closing
// time: the request context ends then, and writes fail from then on. Fails
// closed: a profile that cannot be read is refused.
func gateProfile(w http.ResponseWriter, r *http.Request, claims *domainuser.Claims) (http.ResponseWriter, *http.Request, bool) {
	h := profileGate.Load()
	if h == nil || claims == nil || claims.ProfileID == "" || allowedWhileClosed(r) {
		return w, r, false
	}
	acc, err := h.g.Access(r.Context(), claims.ProfileID)
	if err == nil && acc.Open {
		if acc.ClosesAt.IsZero() {
			return w, r, false
		}
		ctx, cancel := context.WithDeadline(r.Context(), acc.ClosesAt)
		context.AfterFunc(r.Context(), cancel)
		cw := &closingWriter{ResponseWriter: w, deadline: acc.ClosesAt}
		// The network deadline is what stops a write already blocked on a
		// slow client: neither the context nor the per-write check can.
		if dlErr := cw.SetWriteDeadline(acc.ClosesAt); dlErr != nil {
			slog.DebugContext(r.Context(), "profile closing: no write deadline on this connection", slog.Any("err", dlErr))
		}
		return cw, r.WithContext(ctx), false
	}
	if err != nil {
		slog.WarnContext(r.Context(), "profile hours unreadable, request refused", slog.String("profile_id", claims.ProfileID), slog.Any("err", err))
	}
	metrics.AuthzDeniedTotal.WithLabelValues("http", "profile_closed").Inc()
	WriteProfileClosed(w, claims.ProfileID, acc.NextOpen)
	return w, r, true
}

// errProfileClosing ends a response written past its profile's closing time.
var errProfileClosing = errors.New("profile closed during the response")

// closingWriter ends a response at the deadline: a network write deadline
// unblocks a write stuck on a slow client, and writes after the deadline are
// refused, so a copy loop (ServeContent, io.Copy of a transcode) stops and the
// connection closes. Only kids profiles
// with opening hours get it; everyone else keeps the plain writer and its
// sendfile path.
type closingWriter struct {
	http.ResponseWriter
	deadline time.Time
}

func (c *closingWriter) Write(b []byte) (int, error) {
	if !time.Now().Before(c.deadline) {
		return 0, errProfileClosing
	}
	return c.ResponseWriter.Write(b)
}

// SetWriteDeadline is what http.ResponseController calls first: a handler that
// clears the deadline for a long stream (music, direct play: "no deadline")
// or pushes it past the closing time gets the closing time instead. An
// earlier deadline is kept.
func (c *closingWriter) SetWriteDeadline(t time.Time) error {
	if t.IsZero() || t.After(c.deadline) {
		t = c.deadline
	}
	return http.NewResponseController(c.ResponseWriter).SetWriteDeadline(t)
}

// Unwrap lets http.ResponseController reach Flush and read deadlines underneath.
func (c *closingWriter) Unwrap() http.ResponseWriter { return c.ResponseWriter }

func (c *closingWriter) Flush() {
	if f, ok := c.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// WriteProfileClosed writes the profile_closed refusal: the gateway's status
// JSON with an ErrorInfo detail naming the closed profile (a late answer for a
// profile the device has since left must not lock the new one), plus
// X-Evey-Error for players that only see headers.
func WriteProfileClosed(w http.ResponseWriter, profileID string, nextOpen time.Time) {
	meta := map[string]string{"profile_id": profileID}
	if !nextOpen.IsZero() {
		meta["next_open"] = nextOpen.UTC().Format(time.RFC3339)
	}
	body := map[string]any{
		"code":    7,
		"message": "errors.profile.closed",
		"details": []map[string]any{{
			"@type":    "type.googleapis.com/google.rpc.ErrorInfo",
			"reason":   ProfileClosedReason,
			"domain":   "evey",
			"metadata": meta,
		}},
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Evey-Error", "profile_closed")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusForbidden)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		slog.Warn("json encode failed", slog.Any("err", err))
	}
}
