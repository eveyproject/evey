package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"

	otelcodes "go.opentelemetry.io/otel/codes"

	appconfig "github.com/evey-project/evey/internal/app/config"
	"github.com/evey-project/evey/internal/domain/shared"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// SMTPTestSender sends the admin's test message with the SMTP settings in force.
type SMTPTestSender interface {
	SendTest(ctx context.Context, to string) error
}

type smtpTestUserGetter interface {
	GetByID(ctx context.Context, id shared.UserID) (*domainuser.User, error)
}

// writeSMTPTestError answers {"code","message"}: code drives the UI's
// translated text, message carries the SMTP server's own words.
func writeSMTPTestError(ctx context.Context, w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(map[string]string{"code": code, "message": message}); err != nil {
		slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
	}
}

// registerSMTPTestRoute wires the "Send test e-mail" button of the Email / SMTP
// card: a real mail to the signed-in admin's address (or body "to"). Admin only
// and on the heavy limiter, since each call holds an SMTP conversation.
func registerSMTPTestRoute(mux *http.ServeMux, sender SMTPTestSender, users smtpTestUserGetter, tokenGen domainuser.TokenGenerator, heavyRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)
	mux.HandleFunc("POST /api/v1/admin/smtp/test", admin(ratelimit.MiddlewareFunc(heavyRL, func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.smtp.Test")
		defer span.End()

		var body struct {
			To string `json:"to"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 1<<16)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil && !errors.Is(err, io.EOF) {
			writeSMTPTestError(ctx, w, http.StatusBadRequest, "invalid_body", "invalid JSON body")
			return
		}
		to := strings.TrimSpace(body.To)
		if to == "" && users != nil {
			if claims := HTTPClaimsFromContext(ctx); claims != nil {
				if u, err := users.GetByID(ctx, shared.UserID(claims.UserID)); err == nil && u != nil {
					to = strings.TrimSpace(u.Email)
				}
			}
		}
		if to == "" {
			writeSMTPTestError(ctx, w, http.StatusBadRequest, "no_recipient", "your account has no e-mail address")
			return
		}
		addr, err := mail.ParseAddress(to)
		if err != nil {
			writeSMTPTestError(ctx, w, http.StatusBadRequest, "invalid_recipient", err.Error())
			return
		}

		if err := sender.SendTest(ctx, addr.Address); err != nil {
			span.RecordError(err)
			span.SetStatus(otelcodes.Error, "smtp test failed")
			if errors.Is(err, appconfig.ErrSMTPNotConfigured) {
				writeSMTPTestError(ctx, w, http.StatusConflict, "not_configured", err.Error())
				return
			}
			// The operator's SMTP server is what failed, not us: 502 with its reason.
			slog.WarnContext(ctx, "smtp test send failed", slog.Any("err", err))
			writeSMTPTestError(ctx, w, http.StatusBadGateway, "send_failed", err.Error())
			return
		}

		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"to": addr.Address}); err != nil {
			slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
		}
	})))
}
