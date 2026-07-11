package httpapi

import (
	"log/slog"
	"net/http"
	"time"
)

// bodyReadTimeout bounds how long a request may take to deliver its BODY. Go's
// server caps the header read (ReadHeaderTimeout) but nothing caps the body, so
// a client that dribbles a POST body one byte at a time pins a goroutine
// indefinitely - a slow-POST / "slowloris" denial of service. The MaxBytesReader
// size caps limit how MUCH a body may be, not how LONG it may take. 30s is far
// more than any bounded JSON body needs even on a poor link, while killing an
// indefinite stall.
//
// This is deliberately NOT a global http.Server.ReadTimeout: large, legitimately
// slow uploads (2 GB backup restore, 100 MB plugin, avatar) must not be cut off.
// Those handlers call ClearBodyReadDeadline before reading their body.
const bodyReadTimeout = 30 * time.Second

// BodyReadTimeoutMiddleware sets a body read deadline for requests that carry a
// body (POST/PUT/PATCH). Body-less methods (GET/HEAD/DELETE) are left untouched
// so a long-lived streaming GET response never trips a stale read deadline on
// its keep-alive connection.
func BodyReadTimeoutMiddleware(next http.Handler) http.Handler {
	return bodyReadTimeoutMiddleware(next, bodyReadTimeout)
}

// bodyReadTimeoutMiddleware is the duration-parameterised core, so tests can
// exercise the deadline without waiting the production 30s.
func bodyReadTimeoutMiddleware(next http.Handler, d time.Duration) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			if err := http.NewResponseController(w).SetReadDeadline(time.Now().Add(d)); err != nil {
				// Fail open: a ResponseWriter that cannot set a deadline (e.g. a
				// test double) just leaves this request unprotected, as before.
				slog.Debug("body read timeout: set deadline unsupported", slog.Any("err", err))
			}
		}
		next.ServeHTTP(w, r)
	})
}

// ClearBodyReadDeadline removes the deadline set by BodyReadTimeoutMiddleware.
// Large-upload handlers (backup restore, plugin install, avatar) call it before
// reading their body so a legitimately slow multi-hundred-MB upload on a poor
// link is not cut off at bodyReadTimeout.
func ClearBodyReadDeadline(w http.ResponseWriter) {
	if err := http.NewResponseController(w).SetReadDeadline(time.Time{}); err != nil {
		slog.Debug("body read timeout: clear deadline unsupported", slog.Any("err", err))
	}
}
