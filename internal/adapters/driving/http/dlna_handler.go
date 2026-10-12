package httpapi

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appdlna "github.com/eveyproject/evey/internal/app/dlna"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

const dlnaMaxBodySize = 4096

type dlnaClient interface {
	Discover(ctx context.Context) ([]appdlna.Renderer, error)
	Play(ctx context.Context, rendererID, mediaURL, title string) error
	Stop(ctx context.Context, rendererID string) error
	Pause(ctx context.Context, rendererID string) error
}

func dlnaRenderersHandler(dlnaSvc dlnaClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.dlna.Renderers")
		defer span.End()
		r = r.WithContext(ctx)
		renderers, err := dlnaSvc.Discover(r.Context())
		if err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "discovery failed")
			slog.ErrorContext(ctx, "dlna discover", slog.Any("err", err))
			http.Error(w, `{"message":"discovery failed"}`, http.StatusInternalServerError)
			return
		}
		if renderers == nil {
			renderers = []appdlna.Renderer{}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(renderers); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	}
}

func dlnaPlayHandler(dlnaSvc dlnaClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.dlna.Play")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			RendererID string `json:"rendererId"`
			MediaURL   string `json:"mediaUrl"`
			Title      string `json:"title"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, dlnaMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("renderer_id", body.RendererID))
		if body.RendererID == "" || body.MediaURL == "" {
			span.SetStatus(otelcodes.Error, "rendererId and mediaUrl required")
			http.Error(w, `{"message":"rendererId and mediaUrl required"}`, http.StatusBadRequest)
			return
		}
		if err := dlnaSvc.Play(r.Context(), body.RendererID, body.MediaURL, body.Title); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "play failed")
			slog.ErrorContext(ctx, "dlna play", slog.Any("err", err))
			http.Error(w, `{"message":"play failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func dlnaStopHandler(dlnaSvc dlnaClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.dlna.Stop")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			RendererID string `json:"rendererId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, dlnaMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("renderer_id", body.RendererID))
		if body.RendererID == "" {
			span.SetStatus(otelcodes.Error, "rendererId required")
			http.Error(w, `{"message":"rendererId required"}`, http.StatusBadRequest)
			return
		}
		if err := dlnaSvc.Stop(r.Context(), body.RendererID); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "stop failed")
			slog.ErrorContext(ctx, "dlna stop", slog.Any("err", err))
			http.Error(w, `{"message":"stop failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}

func dlnaPauseHandler(dlnaSvc dlnaClient) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.dlna.Pause")
		defer span.End()
		r = r.WithContext(ctx)
		var body struct {
			RendererID string `json:"rendererId"`
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, dlnaMaxBodySize)).Decode(&body); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "bad request")
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		span.SetAttributes(attribute.String("renderer_id", body.RendererID))
		if body.RendererID == "" {
			span.SetStatus(otelcodes.Error, "rendererId required")
			http.Error(w, `{"message":"rendererId required"}`, http.StatusBadRequest)
			return
		}
		if err := dlnaSvc.Pause(r.Context(), body.RendererID); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "pause failed")
			slog.ErrorContext(ctx, "dlna pause", slog.Any("err", err))
			http.Error(w, `{"message":"pause failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":"ok"}`))
	}
}
