package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"go.opentelemetry.io/otel/attribute"
	otelcodes "go.opentelemetry.io/otel/codes"

	appwebhook "github.com/eveyproject/evey/internal/app/webhook"
	"github.com/eveyproject/evey/internal/domain/shared"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// registerWebhookRoutes wires the "Test" button both admin UIs (web + mobile)
// already call: POST /api/v1/admin/webhooks/{id}/test. Every other webhook CRUD
// route is served by grpc-gateway straight from the proto definition; this one
// has no proto RPC (adding one would touch generated code, out of scope here),
// so it is a manual handler. It delegates everything - config lookup, HMAC
// signing, retries, delivery audit - to the same appwebhook.Service grpc-gateway
// uses, so a test ping behaves exactly like a real delivery.
func registerWebhookRoutes(mux *http.ServeMux, svc *appwebhook.Service, tokenGen domainuser.TokenGenerator, heavyRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	// A test ping sends a real outbound HTTP request to an operator-chosen URL,
	// so it must sit behind the same admin gate as every other
	// /api/v1/admin/... route - an ungated test button would be an open SSRF
	// probe. Rate-limited via AdminHeavyRL since each call can block for the
	// full retry/timeout budget of a real delivery.
	mux.HandleFunc("POST /api/v1/admin/webhooks/{id}/test", admin(ratelimit.MiddlewareFunc(heavyRL, func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.webhook.Test")
		defer span.End()
		id := r.PathValue("id")
		span.SetAttributes(attribute.String("webhook_id", id))

		result, err := svc.Test(ctx, id)
		if err != nil {
			span.RecordError(err)
			switch {
			case errors.Is(err, shared.ErrNotFound):
				span.SetStatus(otelcodes.Error, "webhook not found")
				http.Error(w, `{"message":"webhook not found"}`, http.StatusNotFound)
			case errors.Is(err, appwebhook.ErrDeliveryFailed):
				// Not a server fault: the operator's configured URL is what
				// failed. Surfaced as 502 with the actual reason so the admin
				// UI's toast can say WHY the test failed, not just that it did.
				span.SetStatus(otelcodes.Error, "webhook test delivery failed")
				slog.WarnContext(ctx, "webhook test delivery failed", slog.String("webhook_id", id), slog.Any("err", err))
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadGateway)
				if encErr := json.NewEncoder(w).Encode(map[string]string{"message": err.Error()}); encErr != nil {
					slog.WarnContext(ctx, "json encode failed", slog.Any("err", encErr))
				}
			default:
				span.SetStatus(otelcodes.Error, "webhook test failed")
				internalError(ctx, w, "webhook test failed", err, slog.String("webhook_id", id))
			}
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if encErr := json.NewEncoder(w).Encode(map[string]any{
			"statusCode": result.StatusCode,
			"durationMs": result.DurationMS,
		}); encErr != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", encErr))
		}
	})))
}
