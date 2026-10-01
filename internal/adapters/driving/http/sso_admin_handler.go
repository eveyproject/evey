package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"time"

	otelcodes "go.opentelemetry.io/otel/codes"

	appconfig "github.com/evey-project/evey/internal/app/config"
	appuser "github.com/evey-project/evey/internal/app/user"
	domainlibrary "github.com/evey-project/evey/internal/domain/library"
	domainuser "github.com/evey-project/evey/internal/domain/user"
	"github.com/evey-project/evey/internal/pkg/ratelimit"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// ssoSettingsStore persists the panel's SSO keys (appconfig.Service).
type ssoSettingsStore interface {
	Set(ctx context.Context, key, value string) error
}

// ssoLibraryLookup validates the default library selection.
type ssoLibraryLookup interface {
	GetLibrary(ctx context.Context, id string) (*domainlibrary.Library, error)
}

// ssoUpdateBody is the PUT payload; an absent field is left unchanged.
type ssoUpdateBody struct {
	Enabled             *bool     `json:"enabled"`
	Issuer              *string   `json:"issuer"`
	ClientID            *string   `json:"clientId"`
	ClientSecret        *string   `json:"clientSecret"`
	AllowedEmailDomains *[]string `json:"allowedEmailDomains"`
	ButtonLabel         *string   `json:"buttonLabel"`
	LinkByEmail         *bool     `json:"linkByEmail"`
	AllowSignup         *bool     `json:"allowSignup"`
	// DefaultLibraryIDs: empty = every library.
	DefaultLibraryIDs *[]string `json:"defaultLibraryIds"`
}

func ssoPayload(cfg appuser.OIDCConfig, st appuser.OIDCStatus) map[string]any {
	source := "none"
	switch {
	case cfg.FromPanel:
		source = "panel"
	case cfg.Enabled || cfg.Issuer != "":
		source = "env"
	}
	libs := make([]string, 0, len(cfg.DefaultLibraryIDs))
	for _, id := range cfg.DefaultLibraryIDs {
		libs = append(libs, string(id))
	}
	domains := cfg.AllowedEmailDomains
	if domains == nil {
		domains = []string{}
	}
	status := map[string]any{"state": st.State, "error": st.Error, "endSessionEndpoint": st.EndSessionEndpoint}
	if !st.CheckedAt.IsZero() {
		status["checkedAt"] = st.CheckedAt.UTC().Format(time.RFC3339)
	}
	return map[string]any{
		"enabled":               cfg.Enabled,
		"issuer":                cfg.Issuer,
		"clientId":              cfg.ClientID,
		"clientSecret":          appconfig.MaskSecretValue(appuser.OIDCKeyClientSecret, cfg.ClientSecret),
		"allowedEmailDomains":   domains,
		"buttonLabel":           cfg.ButtonLabel,
		"linkByEmail":           cfg.LinkByEmail,
		"allowSignup":           cfg.AllowSignup,
		"allowSignupDefault":    cfg.AllowSignupDefault,
		"defaultLibraryIds":     libs,
		"redirectUri":           cfg.RedirectURL,
		"postLogoutRedirectUri": cfg.PostLogoutRedirectURL,
		"source":                source,
		"status":                status,
	}
}

// validateIssuer accepts an absolute http(s) URL without query or fragment,
// which is what OpenID discovery requires of an issuer identifier.
func validateIssuer(raw string) error {
	if raw == "" {
		return nil
	}
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return fmt.Errorf("must be an absolute http(s) URL")
	}
	if u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("must not carry a query or a fragment")
	}
	return nil
}

func validateEmailDomain(d string) error {
	if len(d) > 253 || !strings.Contains(d, ".") || strings.ContainsAny(d, " @/:") {
		return fmt.Errorf("%q is not a domain", d)
	}
	return nil
}

// ssoWrites validates the body and returns the keys to persist.
func ssoWrites(ctx context.Context, body ssoUpdateBody, libs ssoLibraryLookup) (map[string]string, error) {
	out := map[string]string{}
	if body.Enabled != nil {
		out[appuser.OIDCKeyEnabled] = fmt.Sprint(*body.Enabled)
	}
	if body.Issuer != nil {
		issuer := strings.TrimSpace(*body.Issuer)
		if err := validateIssuer(issuer); err != nil {
			return nil, fmt.Errorf("issuer %w", err)
		}
		out[appuser.OIDCKeyIssuer] = issuer
	}
	if body.ClientID != nil {
		id := strings.TrimSpace(*body.ClientID)
		if len(id) > 512 {
			return nil, fmt.Errorf("client ID is too long")
		}
		out[appuser.OIDCKeyClientID] = id
	}
	// The mask coming back means "unchanged": the secret is never sent out.
	if body.ClientSecret != nil && !appconfig.ShouldKeepStoredSecret(appuser.OIDCKeyClientSecret, *body.ClientSecret) {
		if len(*body.ClientSecret) > 1024 {
			return nil, fmt.Errorf("client secret is too long")
		}
		out[appuser.OIDCKeyClientSecret] = strings.TrimSpace(*body.ClientSecret)
	}
	if body.AllowedEmailDomains != nil {
		domains := appuser.ParseEmailDomains(strings.Join(*body.AllowedEmailDomains, ","))
		for _, d := range domains {
			if err := validateEmailDomain(d); err != nil {
				return nil, fmt.Errorf("allowed e-mail domains: %w", err)
			}
		}
		out[appuser.OIDCKeyAllowedEmailDomains] = strings.Join(domains, ",")
	}
	if body.ButtonLabel != nil {
		label := strings.TrimSpace(*body.ButtonLabel)
		if len([]rune(label)) > 64 {
			return nil, fmt.Errorf("button label is too long (64 characters at most)")
		}
		out[appuser.OIDCKeyButtonLabel] = label
	}
	if body.LinkByEmail != nil {
		out[appuser.OIDCKeyLinkByEmail] = fmt.Sprint(*body.LinkByEmail)
	}
	if body.AllowSignup != nil {
		out[appuser.OIDCKeyAllowSignup] = fmt.Sprint(*body.AllowSignup)
	}
	if body.DefaultLibraryIDs != nil {
		ids := make([]string, 0, len(*body.DefaultLibraryIDs))
		for _, raw := range *body.DefaultLibraryIDs {
			id := strings.TrimSpace(raw)
			if id == "" {
				continue
			}
			if strings.Contains(id, ",") || libs == nil {
				return nil, fmt.Errorf("unknown library %q", id)
			}
			if _, err := libs.GetLibrary(ctx, id); err != nil {
				return nil, fmt.Errorf("unknown library %q", id)
			}
			ids = append(ids, id)
		}
		value := appuser.OIDCDefaultLibrariesAll
		if len(ids) > 0 {
			value = strings.Join(ids, ",")
		}
		out[appuser.OIDCKeyDefaultLibraries] = value
	}
	return out, nil
}

func registerSSOAdminRoutes(mux *http.ServeMux, oidcSvc *appuser.OIDCService, store ssoSettingsStore, libs ssoLibraryLookup, tokenGen domainuser.TokenGenerator, writeRL *ratelimit.Limiter) {
	admin := httpAdminMiddleware(tokenGen)

	writeJSON := func(w http.ResponseWriter, r *http.Request, v any) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		if err := json.NewEncoder(w).Encode(v); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}

	mux.HandleFunc("GET /api/v1/admin/sso", admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.sso.Get")
		defer span.End()
		writeJSON(w, r, ssoPayload(oidcSvc.Status(ctx, false)))
	}))

	mux.HandleFunc("PUT /api/v1/admin/sso", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.sso.Update")
		defer span.End()
		var body ssoUpdateBody
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			span.SetStatus(otelcodes.Error, "bad request")
			messageError(w, "bad request", http.StatusBadRequest)
			return
		}
		writes, err := ssoWrites(ctx, body, libs)
		if err != nil {
			span.SetStatus(otelcodes.Error, "invalid settings")
			messageError(w, err.Error(), http.StatusBadRequest)
			return
		}
		if store == nil {
			http.Error(w, `{"message":"settings store unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		for key, value := range writes {
			if err := store.Set(ctx, key, value); err != nil {
				span.RecordError(err)
				logInternal(ctx, "sso settings: persist "+key, err)
				http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
				return
			}
		}
		// Applies now: the next login re-reads the settings and rediscovers.
		oidcSvc.Invalidate()
		slog.InfoContext(ctx, "sso settings updated", slog.Int("keys", len(writes)))
		writeJSON(w, r, ssoPayload(oidcSvc.Status(ctx, true)))
	})))

	mux.HandleFunc("POST /api/v1/admin/sso/test", ratelimit.MiddlewareFunc(writeRL, admin(func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.sso.Test")
		defer span.End()
		var body struct {
			Issuer string `json:"issuer"`
		}
		r.Body = http.MaxBytesReader(w, r.Body, 8<<10)
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			messageError(w, "bad request", http.StatusBadRequest)
			return
		}
		issuer := strings.TrimSpace(body.Issuer)
		if err := validateIssuer(issuer); err != nil {
			messageError(w, "issuer "+err.Error(), http.StatusBadRequest)
			return
		}
		cfg, cfgErr := oidcSvc.Config(ctx)
		if cfgErr != nil {
			logInternal(ctx, "sso test: load settings", cfgErr)
			http.Error(w, `{"message":"internal error"}`, http.StatusInternalServerError)
			return
		}
		if issuer == "" {
			issuer = cfg.Issuer
		}
		var endSession string
		var testErr error
		// The saved, enabled issuer is refreshed in place so the login page
		// follows the result; any other value is only probed.
		if cfg.Enabled && issuer == cfg.Issuer {
			_, st := oidcSvc.Status(ctx, true)
			endSession = st.EndSessionEndpoint
			if st.State != "ok" {
				testErr = fmt.Errorf("%s", st.Error)
			}
		} else {
			endSession, testErr = oidcSvc.Probe(ctx, issuer)
		}
		out := map[string]any{"ok": testErr == nil, "issuer": issuer, "endSessionEndpoint": endSession}
		if testErr != nil {
			span.SetStatus(otelcodes.Error, "discovery failed")
			out["error"] = testErr.Error()
		}
		writeJSON(w, r, out)
	})))
}
