package user

import (
	"context"
	"fmt"
	"strings"

	"github.com/evey-project/evey/internal/domain/shared"
)

// SSO settings persisted by Admin -> Server settings -> Single sign-on. A key
// present in the store wins over its EVEY_OIDC_* twin, even when empty: the
// panel is the primary source, the environment only bootstraps.
const (
	OIDCKeyEnabled             = "oidc.enabled"
	OIDCKeyIssuer              = "oidc.issuer"
	OIDCKeyClientID            = "oidc.client_id"
	OIDCKeyClientSecret        = "oidc.client_secret" //nolint:gosec // G101: a config key name, masked by appconfig.IsSecretKey
	OIDCKeyAllowedEmailDomains = "oidc.allowed_email_domains"
	OIDCKeyButtonLabel         = "oidc.button_label"
	OIDCKeyLinkByEmail         = "oidc.link_by_email"
	OIDCKeyAllowSignup         = "oidc.allow_signup"
	OIDCKeyDefaultLibraries    = "oidc.default_libraries"
)

// OIDCConfigKeys lists every persisted SSO key, for change detection.
var OIDCConfigKeys = map[string]bool{
	OIDCKeyEnabled: true, OIDCKeyIssuer: true, OIDCKeyClientID: true, OIDCKeyClientSecret: true,
	OIDCKeyAllowedEmailDomains: true, OIDCKeyButtonLabel: true, OIDCKeyLinkByEmail: true,
	OIDCKeyAllowSignup: true, OIDCKeyDefaultLibraries: true,
}

// OIDCCallbackPath and OIDCPostLogoutPath are the URIs the admin registers at
// the provider, relative to the server's public base URL.
const (
	OIDCCallbackPath   = "/api/v1/auth/oidc/callback"
	OIDCPostLogoutPath = "/login"
)

// OIDCDefaultLibrariesAll is the stored value for "every library".
const OIDCDefaultLibrariesAll = "all"

// OIDCSettingsSource resolves the live SSO configuration: panel keys from the
// server config store, then the environment.
type OIDCSettingsSource struct {
	Env OIDCConfig
	// EnvAllowSignup is EVEY_OIDC_ALLOW_SIGNUP raw: "" keeps the default.
	EnvAllowSignup string
	Store          interface {
		List(ctx context.Context) ([]*shared.ServerConfigEntry, error)
	}
	BaseURL func() string
}

// Load reads the store once and resolves every field.
func (src OIDCSettingsSource) Load(ctx context.Context) (OIDCConfig, error) {
	panel := map[string]string{}
	if src.Store != nil {
		entries, err := src.Store.List(ctx)
		if err != nil {
			return OIDCConfig{}, fmt.Errorf("load sso settings: %w", err)
		}
		for _, e := range entries {
			if OIDCConfigKeys[e.Key] {
				panel[e.Key] = e.Value
			}
		}
	}
	base := ""
	if src.BaseURL != nil {
		base = src.BaseURL()
	}
	return ResolveOIDCConfig(src.Env, src.EnvAllowSignup, panel, base), nil
}

// ResolveOIDCConfig merges panel values over the environment. Pure, so the
// precedence and the defaults are testable without a store.
func ResolveOIDCConfig(env OIDCConfig, envAllowSignup string, panel map[string]string, baseURL string) OIDCConfig {
	cfg := env
	cfg.FromPanel = len(panel) > 0
	str := func(key string, dst *string) {
		if v, ok := panel[key]; ok {
			*dst = strings.TrimSpace(v)
		}
	}
	if v, ok := panel[OIDCKeyEnabled]; ok {
		cfg.Enabled = parseBool(v, false)
	}
	str(OIDCKeyIssuer, &cfg.Issuer)
	str(OIDCKeyClientID, &cfg.ClientID)
	str(OIDCKeyClientSecret, &cfg.ClientSecret)
	str(OIDCKeyButtonLabel, &cfg.ButtonLabel)
	if v, ok := panel[OIDCKeyAllowedEmailDomains]; ok {
		cfg.AllowedEmailDomains = ParseEmailDomains(v)
	}
	cfg.LinkByEmail = true
	if v, ok := panel[OIDCKeyLinkByEmail]; ok {
		cfg.LinkByEmail = parseBool(v, true)
	}
	// Open sign-up only behind a domain allowlist by default: with a public
	// provider and no list, anybody holding an account there would get one here.
	cfg.AllowSignup = len(cfg.AllowedEmailDomains) > 0
	cfg.AllowSignupDefault = cfg.AllowSignup
	if v, ok := panel[OIDCKeyAllowSignup]; ok && strings.TrimSpace(v) != "" {
		cfg.AllowSignup = parseBool(v, cfg.AllowSignup)
	} else if envAllowSignup != "" {
		cfg.AllowSignup = parseBool(envAllowSignup, cfg.AllowSignup)
	}
	cfg.DefaultLibraryIDs = nil
	if v, ok := panel[OIDCKeyDefaultLibraries]; ok {
		cfg.DefaultLibraryIDs = ParseLibraryIDs(v)
	}
	if cfg.RedirectURL == "" && baseURL != "" {
		cfg.RedirectURL = strings.TrimRight(baseURL, "/") + OIDCCallbackPath
	}
	cfg.PostLogoutRedirectURL = ""
	if baseURL != "" {
		cfg.PostLogoutRedirectURL = strings.TrimRight(baseURL, "/") + OIDCPostLogoutPath
	}
	return cfg
}

// ParseEmailDomains splits a comma/space separated list, lower-cased, "@" dropped.
func ParseEmailDomains(raw string) []string {
	var out []string
	for _, f := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\n' || r == ';' }) {
		d := strings.ToLower(strings.TrimPrefix(strings.TrimSpace(f), "@"))
		if d != "" {
			out = append(out, d)
		}
	}
	return out
}

// ParseLibraryIDs reads the default-access value: "" or "all" means every
// library (nil), anything else is a comma separated id list.
func ParseLibraryIDs(raw string) []shared.LibraryID {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.EqualFold(raw, OIDCDefaultLibrariesAll) {
		return nil
	}
	var out []shared.LibraryID
	for f := range strings.SplitSeq(raw, ",") {
		if id := strings.TrimSpace(f); id != "" {
			out = append(out, shared.LibraryID(id))
		}
	}
	return out
}

func parseBool(v string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	}
	return def
}
