package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	appuser "github.com/gravity-zero/evey/internal/app/user"
	"github.com/gravity-zero/evey/internal/config"
	domainuser "github.com/gravity-zero/evey/internal/domain/user"
	"github.com/gravity-zero/evey/internal/pkg/quickconnecthub"
)

type qcCheckerAdapter struct {
	query *appuser.QueryService
	auth  *appuser.CommandService
	cfg   *config.Config
}

func (a *qcCheckerAdapter) checkAndGenerateTokens(ctx context.Context, code string, persistent bool) (string, string, bool, error) {
	qc, err := a.query.CheckQuickConnect(ctx, code)
	if err != nil {
		return "", "", false, err
	}
	if qc.Status != string(domainuser.Authorized) || qc.UserID == nil {
		return "", "", false, nil
	}
	var ttlOverride []time.Duration
	if persistent {
		ttlOverride = []time.Duration{a.cfg.JWT.RefreshTokenLongTTL}
	}
	access, refresh, err := a.auth.GenerateTokensForUser(ctx, *qc.UserID, ttlOverride...)
	if err != nil {
		return "", "", false, err
	}
	return access, refresh, true, nil
}

func quickConnectSSEHandler(hub *quickconnecthub.Hub, checker *qcCheckerAdapter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		code := r.PathValue("code")
		if code == "" {
			http.Error(w, `{"message":"missing code"}`, http.StatusBadRequest)
			return
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			http.Error(w, `{"message":"streaming unsupported"}`, http.StatusInternalServerError)
			return
		}

		persistent := strings.EqualFold(r.Header.Get("X-Persistent-Session"), "true")

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
				access, refresh, authorized, err := checker.checkAndGenerateTokens(r.Context(), code, persistent)
				if err != nil || !authorized {
					slog.Warn("quickconnect sse: check failed after notify", slog.Any("err", err))
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
