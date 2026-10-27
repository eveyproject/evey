package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appuser "github.com/eveyproject/evey/internal/app/user"
	"github.com/eveyproject/evey/internal/config"
	"github.com/eveyproject/evey/internal/pkg/quickconnecthub"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

type qcTokenChecker interface {
	checkAndGenerateTokens(ctx context.Context, code, secret string, persistent bool) (string, string, bool, error)
}

type qcCheckerAdapter struct {
	query *appuser.QueryService
	auth  *appuser.CommandService
	cfg   *config.Config
}

func (a *qcCheckerAdapter) checkAndGenerateTokens(ctx context.Context, code, secret string, persistent bool) (string, string, bool, error) {
	// Claimed, not just read: two collectors on one code mint one session.
	qc, err := a.auth.ClaimQuickConnect(ctx, code, secret)
	if err != nil {
		return "", "", false, err
	}
	if qc == nil {
		return "", "", false, nil
	}
	// What the device asked for when it STARTED the pairing decides the
	// lifetime: an EventSource carries no headers, so this stream could never
	// say it on its own and every browser pairing fell back to a one-week
	// session. The header still counts where a caller can set it.
	var ttlOverride []time.Duration
	if persistent || qc.Persistent {
		ttlOverride = []time.Duration{a.cfg.JWT.RefreshTokenLongTTL}
	}
	access, refresh, err := a.auth.GenerateTokensForUser(ctx, string(*qc.UserID), ttlOverride...)
	if err != nil {
		return "", "", false, err
	}
	return access, refresh, true, nil
}

func quickConnectSSEHandler(hub *quickconnecthub.Hub, checker qcTokenChecker) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Span only covers the SSE connection open, not per-event work - long-lived
		// SSE streams shouldn't keep one span open for minutes.
		_, span := tracing.StartSpan(r.Context(), "http.quickconnect.SSEOpen")
		code := r.PathValue("code")
		span.SetAttributes(attribute.String("qc_code", code))
		if code == "" {
			span.SetStatus(otelcodes.Error, "missing code")
			span.End()
			http.Error(w, `{"message":"missing code"}`, http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			span.SetStatus(otelcodes.Error, "streaming unsupported")
			span.End()
			http.Error(w, `{"message":"streaming unsupported"}`, http.StatusInternalServerError)
			return
		}
		// End the open-span here - the loop below is the long-lived SSE stream.
		span.End()

		persistent := strings.EqualFold(r.Header.Get("X-Persistent-Session"), "true")
		// Normally lifted into the header by the redaction middleware; the
		// query is read for a stream that reached this handler without it.
		secret := r.Header.Get(quickConnectSecretHeader)
		if secret == "" {
			secret = r.URL.Query().Get("secret")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Cache-Control", "no-cache")
		w.Header().Set("X-Accel-Buffering", "no")
		w.Header().Set("Connection", "keep-alive")

		ch := hub.Subscribe(code)
		defer hub.Unsubscribe(code, ch)

		ping := time.NewTicker(30 * time.Second)
		defer ping.Stop()

		timeout := time.NewTimer(2*time.Minute + 10*time.Second)
		defer timeout.Stop()

		fmt.Fprintf(w, ": ping\n\n")
		flusher.Flush()

		for {
			select {
			case <-r.Context().Done():
				return
			case <-timeout.C:
				fmt.Fprintf(w, "data: {\"event\":\"expired\"}\n\n")
				flusher.Flush()
				return
			case <-ping.C:
				fmt.Fprintf(w, ": ping\n\n")
				flusher.Flush()
			case <-ch:
				access, refresh, authorized, err := checker.checkAndGenerateTokens(r.Context(), code, secret, persistent)
				if err != nil || !authorized {
					slog.WarnContext(r.Context(), "quickconnect sse: check failed after notify", slog.Any("err", err))
					fmt.Fprintf(w, "data: {\"event\":\"error\"}\n\n")
					flusher.Flush()
					return
				}
				payload, _ := json.Marshal(map[string]any{
					"event":        "authorized",
					"accessToken":  access,
					"refreshToken": refresh,
				})
				fmt.Fprintf(w, "data: %s\n\n", payload)
				flusher.Flush()
				return
			}
		}
	}
}

// withQuickConnectSecretQuery gives the gateway the secret a polling client
// sent as a header (or that the redaction middleware lifted out of the URL):
// the generated route only binds its request from the query string.
func withQuickConnectSecretQuery(r *http.Request) *http.Request {
	sec := r.Header.Get(quickConnectSecretHeader)
	if sec == "" || r.URL.Query().Get("secret") != "" {
		return r
	}
	r2 := r.Clone(r.Context())
	q := r2.URL.Query()
	q.Set("secret", sec)
	r2.URL.RawQuery = q.Encode()
	return r2
}
