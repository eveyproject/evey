package httpapi

import (
	"encoding/json"
	"errors"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"io"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"
	"golang.org/x/net/websocket"

	appsyncplay "github.com/eveyproject/evey/internal/app/syncplay"
	domainsyncplay "github.com/eveyproject/evey/internal/domain/syncplay"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// Device routes power the "Play On" feature: first-party clients (TV app,
// mobile, another browser tab) hold a persistent WebSocket and register as
// controllable targets; the web UI lists them and pushes playback commands
// the server relays to the addressed device. No casting protocol, no LAN
// dependency - it works for remote clients exactly as for local ones.

const deviceCmdMaxBody = 4 << 10 // 4 KiB - commands are tiny JSON

// deviceWSMaxFrameBytes caps one inbound WebSocket frame on the device
// channel. Both the register frame and any future client ack/status frame
// are tiny JSON, but x/net/websocket otherwise applies its 32MB default -
// any connected device could make the server buffer/decode oversized
// payloads at will. Same cap and same rationale as SyncPlay's
// syncPlayMaxFrameBytes (syncplay_handler.go).
const deviceWSMaxFrameBytes = 8 << 10

// deviceWSRegisterWait is how long a freshly upgraded device socket may stay
// silent before the server gives up on its register frame. Atomic so a test
// can shorten it while hijacked socket goroutines it cannot join still read it.
var deviceWSRegisterWait atomic.Int64

func init() { deviceWSRegisterWait.Store(int64(10 * time.Second)) }

type devicesHandler struct {
	registry *appsyncplay.DeviceRegistry
	tokenGen domainuser.TokenGenerator
}

func registerDeviceRoutes(mux *http.ServeMux, registry *appsyncplay.DeviceRegistry, tokenGen domainuser.TokenGenerator) {
	h := &devicesHandler{registry: registry, tokenGen: tokenGen}
	authMW := httpAuthMiddleware(tokenGen)
	mux.HandleFunc("GET /api/v1/devices", authMW(h.list))
	mux.HandleFunc("POST /api/v1/devices/{id}/command", authMW(h.command))
	// WebSocket auth rides Sec-WebSocket-Protocol (extractWSToken), so it
	// can't go through the standard bearer middleware.
	mux.Handle("/api/v1/devices/ws", http.HandlerFunc(h.wsUpgradeHandler))
}

// GET /api/v1/devices - the caller's currently-connected Play On targets.
func (h *devicesHandler) list(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.devices.List")
	defer span.End()
	claims := HTTPClaimsFromContext(ctx)
	devices := h.registry.List(claims.UserID)
	// Project to a stable JSON shape - the domain struct carries UserID,
	// which the client neither needs nor should see echoed back.
	type deviceDTO struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Type        string `json:"type"`
		ConnectedAt string `json:"connectedAt"`
	}
	out := make([]deviceDTO, 0, len(devices))
	for _, d := range devices {
		out = append(out, deviceDTO{
			ID:          d.ID,
			Name:        d.Name,
			Type:        d.Type,
			ConnectedAt: d.ConnectedAt.Format(time.RFC3339),
		})
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]any{"devices": out}); err != nil {
		slog.WarnContext(r.Context(), "devices list: json encode", slog.Any("err", err))
	}
}

// POST /api/v1/devices/{id}/command - push a playback command to one of
// the caller's devices. 404 when the device isn't connected so the UI can
// refresh its list; the registry is scoped per-user, so a caller can only
// ever address their own devices.
func (h *devicesHandler) command(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.devices.Command")
	defer span.End()
	claims := HTTPClaimsFromContext(ctx)
	deviceID := r.PathValue("id")
	span.SetAttributes(attribute.String("device_id", deviceID))

	var cmd domainsyncplay.DeviceCommand
	if err := json.NewDecoder(io.LimitReader(r.Body, deviceCmdMaxBody)).Decode(&cmd); err != nil {
		span.SetStatus(otelcodes.Error, "bad request")
		http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
		return
	}
	if cmd.Type == "" {
		http.Error(w, `{"message":"command type required"}`, http.StatusBadRequest)
		return
	}

	err := h.registry.SendCommand(claims.UserID, deviceID, cmd)
	if errors.Is(err, appsyncplay.ErrDeviceNotConnected) {
		span.SetStatus(otelcodes.Error, "device not connected")
		http.Error(w, `{"message":"device not connected"}`, http.StatusNotFound)
		return
	}
	if err != nil {
		span.RecordError(err)
		span.SetStatus(otelcodes.Error, "send failed")
		slog.WarnContext(r.Context(), "devices command: send", slog.String("device", deviceID), slog.Any("err", err))
		http.Error(w, `{"message":"send failed"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"ok"}`))
}

// wsUpgradeHandler accepts a device's persistent WebSocket. The client's
// first frame must be a DeviceRegisterRequest; after that the connection
// stays open purely to receive server-pushed DeviceCommands. The read
// loop only exists to detect disconnect (and absorb future client acks).
func (h *devicesHandler) wsUpgradeHandler(w http.ResponseWriter, r *http.Request) {
	_, span := tracing.StartSpan(r.Context(), "http.devices.WSUpgrade")
	token := extractWSToken(r)
	claims, vErr := h.tokenGen.ValidateAccessToken(token)
	if vErr != nil {
		span.RecordError(vErr)
		span.SetStatus(otelcodes.Error, "invalid token")
		span.End()
		http.Error(w, `{"message":"unauthenticated"}`, http.StatusUnauthorized)
		return
	}
	userID := claims.UserID
	span.End()

	websocket.Handler(func(ws *websocket.Conn) {
		sender := &wsSender{ws: ws}
		ws.MaxPayloadBytes = deviceWSMaxFrameBytes

		// First frame: registration. Reject the connection if it doesn't
		// arrive promptly or isn't a valid register request.
		// The warning names the client and how long it stayed silent: prod
		// showed bursts of "i/o timeout" here with no way to tell a phone
		// from a browser tab, nor a frozen client from a lost frame.
		upgradedAt := time.Now()
		_ = ws.SetReadDeadline(upgradedAt.Add(time.Duration(deviceWSRegisterWait.Load()))) //nolint:errcheck // best-effort deadline refresh
		var raw string
		if err := websocket.Message.Receive(ws, &raw); err != nil {
			slog.WarnContext(r.Context(), "devices ws: no register frame",
				slog.String("user", userID),
				slog.String("user_agent", r.UserAgent()),
				slog.String("client_ip", ratelimit.ClientIP(r)),
				slog.Int64("waited_ms", time.Since(upgradedAt).Milliseconds()),
				slog.Any("err", err))
			return
		}
		var reg domainsyncplay.DeviceRegisterRequest
		if err := json.Unmarshal([]byte(raw), &reg); err != nil || reg.Type != "register" || reg.DeviceID == "" {
			slog.WarnContext(r.Context(), "devices ws: invalid register frame", slog.String("user", userID))
			return
		}

		if _, err := h.registry.Register(userID, reg.DeviceID, reg.Name, reg.DeviceType, sender); err != nil {
			// Past the per-user cap: the socket is already upgraded (HTTP 101),
			// so there's no status code to return - closing the connection is
			// the rejection. The client can retry after dropping a device.
			slog.WarnContext(r.Context(), "devices ws: register rejected", slog.String("user", userID), slog.Any("err", err))
			return
		}
		defer h.registry.Unregister(userID, reg.DeviceID, sender)

		// Keep the connection alive to receive pushed commands. Future
		// client→server frames (acks, status) are read and currently
		// ignored - the loop's job is disconnect detection.
		const readDeadline = 120 * time.Second
		for {
			if err := ws.SetReadDeadline(time.Now().Add(readDeadline)); err != nil {
				return
			}
			var msg string
			if err := websocket.Message.Receive(ws, &msg); err != nil {
				return
			}
		}
	}).ServeHTTP(w, r)
}
