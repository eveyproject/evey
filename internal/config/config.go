package config

import (
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// validateCosignPublicKey checks that the configured cosign public key path
// points to a readable PEM-encoded public key. Run at startup so a typo or
// missing file is surfaced immediately rather than at the first plugin
// install. Accepts standard PEM blocks (PUBLIC KEY) only; cosign also
// supports key references prefixed with k8s://, gcpkms://, etc., which we
// treat as opaque and skip validation for.
func validateCosignPublicKey(pathOrRef string) error {
	if strings.Contains(pathOrRef, "://") {
		// remote KMS reference — cosign resolves at verify time.
		return nil
	}
	data, err := os.ReadFile(pathOrRef)
	if err != nil {
		return fmt.Errorf("cannot read public key file %q: %w", pathOrRef, err)
	}
	if len(data) == 0 {
		return fmt.Errorf("public key file %q is empty", pathOrRef)
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return fmt.Errorf("public key file %q is not a PEM-encoded key", pathOrRef)
	}
	if !strings.Contains(block.Type, "PUBLIC KEY") {
		return fmt.Errorf("public key file %q has unexpected PEM type %q (expected PUBLIC KEY)", pathOrRef, block.Type)
	}
	return nil
}

type Config struct {
	Server       ServerConfig
	DB           DBConfig
	Meilisearch  MeilisearchConfig
	JWT          JWTConfig
	Transcoding  TranscodingConfig
	OIDC         OIDCConfig
	Metrics      MetricsConfig
	Library      LibraryConfig
	Playback     PlaybackConfig
	Metadata     MetadataConfig
	Log          LogConfig
	Backup       BackupConfig
	RemoteAccess RemoteAccessConfig
	SMTP    SMTPConfig
	VAPID   VAPIDConfig
	Tracing TracingConfig
	Plugins PluginsConfig
}

// PluginsConfig holds configuration for the third-party plugin subsystem.
type PluginsConfig struct {
	// SandboxMode controls the isolation level applied to plugin processes.
	// Valid values: "basic" (LD_PRELOAD seccomp + UID drop),
	// "bubblewrap" (bwrap namespace isolation), "strict" (bwrap + strict seccomp).
	// Defaults to "bubblewrap" when unset or empty.
	// EVEY_PLUGINS_SANDBOX_MODE
	SandboxMode string

	// Resource limits applied via cgroup v2 per plugin process.
	// Zero values use the runner package defaults (256 MiB, 50% CPU, 64 PIDs).
	// EVEY_PLUGINS_MEMORY_MB — max RSS in MiB, default 256.
	MemoryMB int64
	// EVEY_PLUGINS_CPU_PERCENT — CPU share in percent (1-100), default 50.
	CPUPercent int
	// EVEY_PLUGINS_PIDS_MAX — max process/thread count per plugin, default 64.
	PIDsMax int

	// CosignPolicy controls whether plugin ZIPs must carry a cosign signature.
	// Values: "off" (no verification), "warn" (default — install proceeds but
	// unsigned/invalid plugins surface a finding in the security dashboard),
	// "enforce" (block install when signature is missing or invalid).
	// EVEY_PLUGINS_COSIGN_POLICY
	CosignPolicy string
	// CosignPublicKey is a path to the PEM public key used to verify signatures.
	// Required when CosignPolicy != "off".
	// EVEY_PLUGINS_COSIGN_PUBLIC_KEY
	CosignPublicKey string
}

type TracingConfig struct {
	OTLPEndpoint string  // EVEY_OTLP_ENDPOINT, empty = disabled
	ServiceName  string  // EVEY_TRACING_SERVICE_NAME, default "evey"
	SampleRatio  float64 // EVEY_TRACING_SAMPLE_RATIO in [0,1], default 0
}

type MetadataConfig struct {
	Language string // ISO 639-1, default "en"
}

type VAPIDConfig struct {
	PublicKey  string // VAPID_PUBLIC_KEY
	PrivateKey string // VAPID_PRIVATE_KEY
	Subject    string // VAPID_SUBJECT
}

type SMTPConfig struct {
	Host     string // EVEY_SMTP_HOST
	Port     int    // EVEY_SMTP_PORT, default 587
	Username string // EVEY_SMTP_USERNAME
	Password string // EVEY_SMTP_PASSWORD
	From     string // EVEY_SMTP_FROM
	TLS      string // EVEY_SMTP_TLS: none, starttls, ssl
}

type BackupConfig struct {
	Dir           string
	IntervalHours int // 0 = disabled
	RetentionDays int // 0 = keep forever
}

type RemoteAccessConfig struct {
	Enabled              bool
	ExternalURL          string
	LANNetworks          []string
	MaxRemoteBitrateKbps int
}

type LogConfig struct {
	Level               string
	Dir                 string
	MaxSizeMB           int
	RetentionDays       int
	PluginRetentionDays int
	DevMode             bool
	PluginDebug         bool
}

type ServerConfig struct {
	Port              int
	BaseURL           string
	Name              string
	WebDir            string
	TrustedProxies    []string
	RealIPHeader      string
	ForwardedProto    bool
	LoginRateLimitMax  int
	GlobalRateLimitMax int
	// PluginsEnabled gates the entire third-party plugin subsystem.
	// Defaults to false for closed-beta installs: the admin RPCs return
	// FailedPrecondition and AutoStartPlugins is skipped at boot. Flip
	// to true once the plugin sandbox + permission model is field-tested.
	PluginsEnabled bool
}

type DBConfig struct {
	Host         string
	Port         int
	User         string
	Password     string
	DBName       string
	SSLMode      string
	MaxOpenConns int
	MaxIdleConns int
}

func (c DBConfig) DSN() string {
	// libpq keyword/value format — values containing spaces, quotes or
	// backslashes are wrapped in single quotes with ' and \ escaped.
	// Without this, a password like `p@ss 'foo` silently breaks the DSN.
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		escapeDSNValue(c.Host),
		c.Port,
		escapeDSNValue(c.User),
		escapeDSNValue(c.Password),
		escapeDSNValue(c.DBName),
		escapeDSNValue(c.SSLMode),
	)
}

func escapeDSNValue(v string) string {
	needsQuote := v == ""
	for _, r := range v {
		if r == ' ' || r == '\'' || r == '\\' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return v
	}
	buf := make([]byte, 0, len(v)+2)
	buf = append(buf, '\'')
	for _, r := range v {
		switch r {
		case '\'', '\\':
			buf = append(buf, '\\', byte(r))
		default:
			buf = append(buf, string(r)...)
		}
	}
	buf = append(buf, '\'')
	return string(buf)
}

type MeilisearchConfig struct {
	URL    string
	APIKey string
}

type JWTConfig struct {
	PrivateKeyPath      string
	AccessTokenTTL      time.Duration
	RefreshTokenTTL     time.Duration
	RefreshTokenLongTTL time.Duration
}

type TranscodingConfig struct {
	MaxConcurrent   int
	CacheMaxGB      int
	CacheDir        string
	HWAccelEnabled  bool
	TrickplayCacheDir string
}

type OIDCConfig struct {
	Enabled             bool
	Issuer              string
	ClientID            string
	ClientSecret        string
	RedirectURL         string
	AllowedEmailDomains []string // EVEY_OIDC_ALLOWED_EMAIL_DOMAINS — comma-separated; empty = allow all
}

type MetricsConfig struct {
	RetentionDays     int
	SampleIntervalSec int
}

type LibraryConfig struct {
	DefaultScanInterval   time.Duration
	PeriodicScanInterval  time.Duration // fallback polling when inotify misses events
	WatcherEnabled        bool
}

type PlaybackConfig struct {
	DefaultMaxStreams   int
	SkipForwardSeconds  int
	SkipBackwardSeconds int
}

func (c *Config) EffectiveBaseURL() string {
	if c.RemoteAccess.ExternalURL != "" {
		return c.RemoteAccess.ExternalURL
	}
	return c.Server.BaseURL
}

func (c *Config) EffectiveServerName() string       { return c.Server.Name }
func (c *Config) EffectiveMaxConcurrent() int        { return c.Transcoding.MaxConcurrent }
func (c *Config) EffectiveHWAccelEnabled() bool      { return c.Transcoding.HWAccelEnabled }

// WarnValidate validates config, returning an error for critical security issues
// and logging warnings for non-critical ones.
// In DevMode the hard errors are downgraded to warnings so dev iteration stays smooth.
func (c *Config) WarnValidate() error {
	dev := c.Log.DevMode
	var errs []string

	hardFail := func(msg string) {
		if dev {
			slog.Warn("config hardening", slog.String("issue", msg), slog.Bool("dev_mode", true))
			return
		}
		errs = append(errs, msg)
	}

	if c.DB.Password == "evey" {
		hardFail("EVEY_DB_PASSWORD is the default 'evey' — set a strong database password")
	}

	if c.DB.SSLMode == "prefer" && !isLoopbackHost(c.DB.Host) {
		slog.Warn("EVEY_DB_SSLMODE=prefer against a non-loopback host — use 'require' or 'verify-full' in production",
			slog.String("host", c.DB.Host))
	}

	k := c.Meilisearch.APIKey
	if k == "" || k == "changeme" || k == "change-me-generate-a-random-secret" {
		hardFail("MEILI_MASTER_KEY is empty or default — set a strong secret (>= 16 chars)")
	} else if len(k) < 16 {
		slog.Warn("meilisearch API key is shorter than 16 chars — use a longer secret")
	}

	if (c.VAPID.PublicKey == "") != (c.VAPID.PrivateKey == "") {
		slog.Warn("VAPID_PUBLIC_KEY and VAPID_PRIVATE_KEY must both be set or both empty — web push will be disabled")
	}

	if c.OIDC.Enabled && c.OIDC.Issuer != "" && c.OIDC.ClientSecret == "" {
		slog.Warn("OIDC enabled but client secret is empty — set EVEY_OIDC_CLIENT_SECRET")
	}

	switch c.Plugins.SandboxMode {
	case "basic", "bubblewrap", "strict":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("cfg.Plugins.SandboxMode: invalid value %q (expected basic|bubblewrap|strict)", c.Plugins.SandboxMode))
	}
	if c.Plugins.SandboxMode == "bubblewrap" || c.Plugins.SandboxMode == "strict" {
		if _, err := exec.LookPath("bwrap"); err != nil {
			slog.Warn("cfg.Plugins.SandboxMode requires bwrap but binary not found in $PATH — plugin runner may fail at runtime",
				slog.String("sandbox_mode", c.Plugins.SandboxMode))
		}
	}
	switch c.Plugins.CosignPolicy {
	case "off", "warn", "enforce":
		// valid
	default:
		errs = append(errs, fmt.Sprintf("cfg.Plugins.CosignPolicy: invalid value %q (expected off|warn|enforce)", c.Plugins.CosignPolicy))
	}
	if c.Plugins.CosignPolicy != "off" {
		if c.Plugins.CosignPublicKey == "" {
			// Default policy is "warn"; without a key the verifier degrades to
			// a no-op (see CosignVerifier.VerifySignature). Flag it so admins
			// know they're not actually verifying anything.
			slog.Warn("EVEY_PLUGINS_COSIGN_POLICY is non-off but EVEY_PLUGINS_COSIGN_PUBLIC_KEY is unset — signature verification is disabled. Set the public key to enable, or set policy=off to silence.",
				slog.String("policy", c.Plugins.CosignPolicy))
		} else {
			// Fail-fast on misconfigured cosign key: in `enforce` mode every
			// install would silently fail later; in `warn` we'd flood logs on
			// each upload. Catch unreadable / non-PEM key files at startup.
			if err := validateCosignPublicKey(c.Plugins.CosignPublicKey); err != nil {
				errs = append(errs, fmt.Sprintf("cfg.Plugins.CosignPublicKey: %v", err))
			}
		}
	}
	if p := c.Plugins.CPUPercent; p < 0 || p > 100 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.CPUPercent: value %d out of range (1-100)", p))
	}
	if m := c.Plugins.MemoryMB; m < 0 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.MemoryMB: value %d must be >= 0 (0 = use default)", m))
	}
	if p := c.Plugins.PIDsMax; p < 0 {
		errs = append(errs, fmt.Sprintf("cfg.Plugins.PIDsMax: value %d must be >= 0 (0 = use default)", p))
	}

	if len(errs) > 0 {
		return fmt.Errorf("config validation failed: %s", strings.Join(errs, "; "))
	}
	return nil
}

func isLoopbackHost(host string) bool {
	switch host {
	case "localhost", "127.0.0.1", "::1", "":
		return true
	}
	return false
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:              envInt("EVEY_SERVER_PORT", 9090),
			BaseURL:           envStr("EVEY_SERVER_BASE_URL", "http://localhost:9091"),
			Name:              envStr("EVEY_SERVER_NAME", "Evey"),
			WebDir:            envStr("EVEY_WEB_DIR", "/srv"),
			TrustedProxies:    envStrSlice("EVEY_TRUSTED_PROXIES", []string{"127.0.0.1/32", "::1/128"}),
			RealIPHeader:      envStr("EVEY_REAL_IP_HEADER", "X-Forwarded-For"),
			ForwardedProto:    envBool("EVEY_FORWARDED_PROTO", false),
			LoginRateLimitMax:  envInt("EVEY_RATELIMIT_LOGIN_MAX", 10),
			GlobalRateLimitMax: envInt("EVEY_RATELIMIT_GLOBAL_MAX", 300),
			PluginsEnabled:     envBool("EVEY_PLUGINS_ENABLED", false),
		},
		DB: DBConfig{
			Host:         envStr("EVEY_DB_HOST", "localhost"),
			Port:         envInt("EVEY_DB_PORT", 5432),
			User:         envStr("EVEY_DB_USER", "evey"),
			Password:     envStr("EVEY_DB_PASSWORD", "evey"),
			DBName:       envStr("EVEY_DB_NAME", "evey"),
			SSLMode:      envStr("EVEY_DB_SSLMODE", "prefer"),
			MaxOpenConns: envInt("EVEY_DB_MAX_OPEN_CONNS", 50),
			MaxIdleConns: envInt("EVEY_DB_MAX_IDLE_CONNS", 20),
		},
		Meilisearch: MeilisearchConfig{
			URL:    envStr("EVEY_MEILI_URL", "http://localhost:7700"),
			APIKey: envStr("MEILI_MASTER_KEY", "changeme"),
		},
		JWT: JWTConfig{
			PrivateKeyPath:      envStr("EVEY_JWT_PRIVATE_KEY_PATH", "/data/jwt.key"),
			AccessTokenTTL:      envDuration("EVEY_JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL:     envDuration("EVEY_JWT_REFRESH_TTL", 7*24*time.Hour),
			RefreshTokenLongTTL: envDuration("EVEY_JWT_REFRESH_LONG_TTL", 180*24*time.Hour),
		},
		Transcoding: TranscodingConfig{
			MaxConcurrent:     envInt("EVEY_TRANSCODING_MAX_CONCURRENT", 2),
			CacheMaxGB:        envInt("EVEY_TRANSCODING_CACHE_MAX_GB", 50),
			CacheDir:          envStr("EVEY_TRANSCODING_CACHE_DIR", "/data/transcode"),
			HWAccelEnabled:    envBool("EVEY_TRANSCODING_HW_ACCEL_ENABLED", false),
			TrickplayCacheDir: envStr("EVEY_TRICKPLAY_CACHE_DIR", "/cache/trickplay"),
		},
		OIDC: OIDCConfig{
			Enabled:             envBool("EVEY_OIDC_ENABLED", false),
			Issuer:              envStr("EVEY_OIDC_ISSUER", ""),
			ClientID:            envStr("EVEY_OIDC_CLIENT_ID", ""),
			ClientSecret:        envStr("EVEY_OIDC_CLIENT_SECRET", ""),
			RedirectURL:         envStr("EVEY_OIDC_REDIRECT_URL", ""),
			AllowedEmailDomains: envStrSlice("EVEY_OIDC_ALLOWED_EMAIL_DOMAINS", nil),
		},
		Metrics: MetricsConfig{
			RetentionDays:     envInt("EVEY_METRICS_RETENTION_DAYS", 90),
			SampleIntervalSec: envInt("EVEY_METRICS_SAMPLE_INTERVAL_SEC", 60),
		},
		Library: LibraryConfig{
			DefaultScanInterval:  envDuration("EVEY_LIBRARY_SCAN_INTERVAL", 6*time.Hour),
			PeriodicScanInterval: envDuration("EVEY_LIBRARY_PERIODIC_SCAN_INTERVAL", 2*time.Minute),
			WatcherEnabled:       envBool("EVEY_LIBRARY_WATCHER_ENABLED", true),
		},
		Playback: PlaybackConfig{
			DefaultMaxStreams:   envInt("EVEY_PLAYBACK_MAX_STREAMS", 0),
			SkipForwardSeconds:  envInt("EVEY_PLAYBACK_SKIP_FORWARD", 10),
			SkipBackwardSeconds: envInt("EVEY_PLAYBACK_SKIP_BACKWARD", 10),
		},
		Metadata: MetadataConfig{Language: "en"},
		Backup: BackupConfig{
			Dir:           envStr("EVEY_BACKUP_DIR", "/data/backups"),
			IntervalHours: envInt("EVEY_BACKUP_INTERVAL_HOURS", 0),
			RetentionDays: envInt("EVEY_BACKUP_RETENTION_DAYS", 30),
		},
		Log: LogConfig{
			Level:               envStr("EVEY_LOG_LEVEL", "info"),
			Dir:                 envStr("EVEY_LOG_DIR", "logs"),
			MaxSizeMB:           envInt("EVEY_LOG_MAX_SIZE_MB", 50),
			RetentionDays:       envInt("EVEY_LOG_RETENTION_DAYS", 90),
			PluginRetentionDays: envInt("EVEY_LOG_PLUGIN_RETENTION_DAYS", 10),
			DevMode:             envBool("EVEY_LOG_DEV_MODE", false),
			PluginDebug:         envBool("EVEY_PLUGIN_DEBUG", false),
		},
		RemoteAccess: RemoteAccessConfig{
			Enabled:              envBool("EVEY_REMOTE_ACCESS_ENABLED", false),
			ExternalURL:          envStr("EVEY_REMOTE_ACCESS_EXTERNAL_URL", ""),
			LANNetworks:          envStrSlice("EVEY_REMOTE_ACCESS_LAN_NETWORKS", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}),
			MaxRemoteBitrateKbps: envInt("EVEY_REMOTE_ACCESS_MAX_BITRATE_KBPS", 0),
		},
		SMTP: SMTPConfig{
			Host:     envStr("EVEY_SMTP_HOST", ""),
			Port:     envInt("EVEY_SMTP_PORT", 587),
			Username: envStr("EVEY_SMTP_USERNAME", ""),
			Password: envStr("EVEY_SMTP_PASSWORD", ""),
			From:     envStr("EVEY_SMTP_FROM", ""),
			TLS:      envStr("EVEY_SMTP_TLS", "starttls"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  envStr("VAPID_PUBLIC_KEY", ""),
			PrivateKey: envStr("VAPID_PRIVATE_KEY", ""),
			Subject:    envStr("VAPID_SUBJECT", "mailto:admin@localhost"),
		},
		Tracing: TracingConfig{
			OTLPEndpoint: envStr("EVEY_OTLP_ENDPOINT", ""),
			ServiceName:  envStr("EVEY_TRACING_SERVICE_NAME", "evey"),
			SampleRatio:  envFloat("EVEY_TRACING_SAMPLE_RATIO", 0),
		},
		Plugins: PluginsConfig{
			SandboxMode:     envPluginSandboxMode("EVEY_PLUGINS_SANDBOX_MODE", "bubblewrap"),
			MemoryMB:        int64(envInt("EVEY_PLUGINS_MEMORY_MB", 256)),
			CPUPercent:      envInt("EVEY_PLUGINS_CPU_PERCENT", 50),
			PIDsMax:         envInt("EVEY_PLUGINS_PIDS_MAX", 64),
			CosignPolicy:    strings.ToLower(strings.TrimSpace(envStr("EVEY_PLUGINS_COSIGN_POLICY", "warn"))),
			CosignPublicKey: envStr("EVEY_PLUGINS_COSIGN_PUBLIC_KEY", ""),
		},
	}
	return cfg, nil
}

// envPluginSandboxMode reads the sandbox mode env var and defaults to fallback
// when unset or empty. The value is lowercased for robustness.
func envPluginSandboxMode(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return strings.ToLower(strings.TrimSpace(v))
	}
	return fallback
}

func envStr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func envInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func envBool(key string, fallback bool) bool {
	if v := os.Getenv(key); v != "" {
		if b, err := strconv.ParseBool(v); err == nil {
			return b
		}
	}
	return fallback
}

func envStrSlice(key string, fallback []string) []string {
	if v := os.Getenv(key); v != "" {
		parts := strings.Split(v, ",")
		result := make([]string, 0, len(parts))
		for _, p := range parts {
			if s := strings.TrimSpace(p); s != "" {
				result = append(result, s)
			}
		}
		return result
	}
	return fallback
}

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}

func envFloat(key string, fallback float64) float64 {
	if v := os.Getenv(key); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			return f
		}
	}
	return fallback
}
