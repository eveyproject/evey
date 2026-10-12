package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appcast "github.com/eveyproject/evey/internal/app/chromecast"
	"github.com/eveyproject/evey/internal/pkg/network"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const castMaxBodySize = 4096

// castClient is the minimal surface of appcast.Service used by chromecast handlers.
// It exists so handlers can be unit-tested with a fake implementation.
type castClient interface {
	Discover(ctx context.Context, clientIP string) ([]appcast.Device, error)
	Play(ctx context.Context, uuid, mediaURL, title, contentType string) error
	Stop(ctx context.Context, uuid string) error
	Pause(ctx context.Context, uuid string) error
	Seek(ctx context.Context, uuid string, positionSec int) error
	Status(ctx context.Context, uuid string) (map[string]any, error)
}

func castDevicesHandler(castSvc castClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.chromecast.Devices")
		defer span.End()
		r = r.WithContext(ctx)
		// The client's IP feeds the unicast subnet-probe fallback: when mDNS
		// multicast is dead (Docker bridge), their own /24 is where their
		// Chromecast lives.
		var clientIP string
		if ip := network.ClientIP(r); ip != nil {
			clientIP = ip.String()
		}
		devices, err := castSvc.Discover(r.Context(), clientIP)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "discovery failed")
			slog.ErrorContext(ctx, "chromecast discover", slog.Any("err", err))
			http.Error(w, `{"message":"discovery failed"}`, http.StatusInternalServerError)
			return
		}
		if devices == nil {
			devices = []appcast.Device{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(devices); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}
}

func castPlayHandler(castSvc castClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.chromecast.Play")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			DeviceUUID  string `json:"deviceUuid"`
			MediaURL    string `json:"mediaUrl"`
			Title       string `json:"title"`
			ContentType string `json:"contentType"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("device_uuid", body.DeviceUUID))
		if body.DeviceUUID == "" || body.MediaURL == "" {
			span.SetStatus(otelcodes.Error, "deviceUuid and mediaUrl required")
			http.Error(w, `{"message":"deviceUuid and mediaUrl required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Play(r.Context(), body.DeviceUUID, body.MediaURL, body.Title, body.ContentType); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "play failed")
			slog.ErrorContext(ctx, "chromecast play", slog.Any("err", err))
			http.Error(w, `{"message":"play failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castStopHandler(castSvc castClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.chromecast.Stop")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			DeviceUUID string `json:"deviceUuid"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("device_uuid", body.DeviceUUID))
		if body.DeviceUUID == "" {
			span.SetStatus(otelcodes.Error, "deviceUuid required")
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Stop(r.Context(), body.DeviceUUID); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "stop failed")
			slog.ErrorContext(ctx, "chromecast stop", slog.Any("err", err))
			http.Error(w, `{"message":"stop failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castPauseHandler(castSvc castClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.chromecast.Pause")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			DeviceUUID string `json:"deviceUuid"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("device_uuid", body.DeviceUUID))
		if body.DeviceUUID == "" {
			span.SetStatus(otelcodes.Error, "deviceUuid required")
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Pause(r.Context(), body.DeviceUUID); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "pause failed")
			slog.ErrorContext(ctx, "chromecast pause", slog.Any("err", err))
			http.Error(w, `{"message":"pause failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castSeekHandler(castSvc castClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.chromecast.Seek")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			DeviceUUID  string `json:"deviceUuid"`
			PositionSec int    `json:"positionSec"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, castMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("device_uuid", body.DeviceUUID))
		if body.DeviceUUID == "" {
			span.SetStatus(otelcodes.Error, "deviceUuid required")
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		if err := castSvc.Seek(r.Context(), body.DeviceUUID, body.PositionSec); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "seek failed")
			slog.ErrorContext(ctx, "chromecast seek", slog.Any("err", err))
			http.Error(w, `{"message":"seek failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func castStatusHandler(castSvc castClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.chromecast.Status")
		defer span.End()
		r = r.WithContext(ctx)
		uuid := r.URL.Query().Get("deviceUuid")
		span.SetAttributes(attribute.String("device_uuid", uuid))
		if uuid == "" {
			span.SetStatus(otelcodes.Error, "deviceUuid required")
			http.Error(w, `{"message":"deviceUuid required"}`, http.StatusBadRequest)
			return
		}
		status, err := castSvc.Status(r.Context(), uuid)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "status failed")
			slog.ErrorContext(ctx, "chromecast status", slog.Any("err", err))
			http.Error(w, `{"message":"status failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(status); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}
}
