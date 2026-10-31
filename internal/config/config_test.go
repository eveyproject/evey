package config

import (
	"bytes"
	"log/slog"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureLogs redirects the default logger for the duration of one test and
// returns the accumulated output. The parsers below warn through slog, and a
// warning nobody can observe is the very thing these tests exist to prevent.
func captureLogs(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	t.Cleanup(func() { slog.SetDefault(prev) })
	return &buf
}

func TestLoad_Defaults(t *testing.T) {
	// Can't run in parallel - we mutate the process env. The dev container
	// pre-sets EVEY_SMTP_*, EVEY_DB_* etc., which would otherwise leak
	// into defaults and break the asserts.
	for _, e := range os.Environ() {
		i := strings.Index(e, "=")
		if i <= 0 {
			continue
		}
		name := e[:i]
		if strings.HasPrefix(name, "EVEY_") ||
			strings.HasPrefix(name, "MEILI_") ||
			strings.HasPrefix(name, "VAPID_") {
			t.Setenv(name, "")
			os.Unsetenv(name)
		}
	}
	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 9090, cfg.Server.Port)
	assert.Equal(t, "http://localhost:9091", cfg.Server.BaseURL)
	assert.Equal(t, "Evey", cfg.Server.Name)
	assert.Equal(t, []string{"127.0.0.1/32", "::1/128"}, cfg.Server.TrustedProxies)
	assert.Equal(t, "X-Forwarded-For", cfg.Server.RealIPHeader)
	assert.False(t, cfg.Server.ForwardedProto)
	assert.Equal(t, 10, cfg.Server.LoginRateLimitMax)

	assert.Equal(t, "localhost", cfg.DB.Host)
	assert.Equal(t, 5432, cfg.DB.Port)
	assert.Equal(t, "evey", cfg.DB.User)
	assert.Equal(t, "evey", cfg.DB.Password)
	assert.Equal(t, "evey", cfg.DB.DBName)
	// Loopback host: TLS is pointless and lib/pq rejects "prefer", so the
	// default is disable - what `docker exec evey admin` needs against the
	// embedded Postgres without any variable set.
	assert.Equal(t, "disable", cfg.DB.SSLMode)
	assert.Equal(t, 25, cfg.DB.MaxOpenConns)
	assert.Equal(t, 10, cfg.DB.MaxIdleConns)

	assert.Equal(t, "http://localhost:7700", cfg.Meilisearch.URL)
	assert.Equal(t, "changeme", cfg.Meilisearch.APIKey)

	assert.Equal(t, "/data/jwt.key", cfg.JWT.PrivateKeyPath)
	assert.Equal(t, 15*time.Minute, cfg.JWT.AccessTokenTTL)
	assert.Equal(t, 7*24*time.Hour, cfg.JWT.RefreshTokenTTL)

	assert.Equal(t, 2, cfg.Transcoding.MaxConcurrent)
	assert.Equal(t, 10, cfg.Transcoding.CacheMaxGB)
	assert.Equal(t, "/data/transcode", cfg.Transcoding.CacheDir)
	assert.False(t, cfg.Transcoding.HWAccelEnabled)
	assert.False(t, cfg.Transcoding.HWPerVariant)
	// Single-variant (encode ONLY the rung the client asked for) is the default:
	// multi-variant also encodes every rung nobody watches, and on prod that was
	// four concurrent libx264 outputs - 2160p included - for one viewer pinned to
	// one rung. Opt back into the shared decode with
	// EVEY_TRANSCODING_SINGLE_VARIANT=0 on a host with no HW decoder.
	assert.True(t, cfg.Transcoding.SingleVariant)
	assert.Equal(t, 0, cfg.Transcoding.MaxSessions) // 0 = runtime.NumCPU()
	assert.Equal(t, "/cache/trickplay", cfg.Transcoding.TrickplayCacheDir)

	assert.False(t, cfg.OIDC.Enabled)
	assert.Empty(t, cfg.OIDC.Issuer)
	assert.Empty(t, cfg.OIDC.ClientID)
	assert.Empty(t, cfg.OIDC.ClientSecret)
	assert.Nil(t, cfg.OIDC.AllowedEmailDomains)

	assert.Equal(t, 90, cfg.Metrics.RetentionDays)
	assert.Equal(t, 60, cfg.Metrics.SampleIntervalSec)

	assert.Equal(t, 2*time.Minute, cfg.Library.PeriodicScanInterval)
	assert.True(t, cfg.Library.WatcherEnabled)
	assert.InDelta(t, 5.0, cfg.Library.PruneMaxMissingPercent, 0)
	assert.Equal(t, 20, cfg.Library.PruneMinMissing)

	assert.Equal(t, 30, cfg.Playback.SkipForwardSeconds)
	assert.Equal(t, 30, cfg.Playback.SkipBackwardSeconds)

	assert.Equal(t, "en", cfg.Metadata.Language)

	assert.Equal(t, "/data/backups", cfg.Backup.Dir)
	assert.Equal(t, 0, cfg.Backup.IntervalHours)
	assert.Equal(t, 30, cfg.Backup.RetentionDays)

	assert.Equal(t, "info", cfg.Log.Level)
	assert.Equal(t, "logs", cfg.Log.Dir)
	assert.Equal(t, 50, cfg.Log.MaxSizeMB)
	assert.Equal(t, 90, cfg.Log.RetentionDays)
	assert.Equal(t, 10, cfg.Log.PluginRetentionDays)
	assert.False(t, cfg.Log.DevMode)

	assert.False(t, cfg.RemoteAccess.Enabled)
	assert.Empty(t, cfg.RemoteAccess.ExternalURL)
	assert.Equal(t, []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"}, cfg.RemoteAccess.LANNetworks)
	assert.Equal(t, 0, cfg.RemoteAccess.MaxRemoteBitrateKbps)

	assert.Equal(t, 587, cfg.SMTP.Port)
	assert.Equal(t, "starttls", cfg.SMTP.TLS)
	assert.Empty(t, cfg.SMTP.Host)

	assert.Empty(t, cfg.VAPID.PublicKey)
	assert.Equal(t, "mailto:admin@localhost", cfg.VAPID.Subject)

	assert.Empty(t, cfg.Tracing.OTLPEndpoint)
	assert.Equal(t, "evey", cfg.Tracing.ServiceName)
	assert.InDelta(t, 0.0, cfg.Tracing.SampleRatio, 0.0001)

	// Plugins defaults: bubblewrap sandbox + cosign warn (non-blocking, surfaces
	// finding when key is configured). Without a key the verifier degrades to
	// a no-op so plugin developers see no behaviour change vs the previous
	// "off" default - only admins who configure a key get verification.
	assert.Equal(t, "auto", cfg.Plugins.SandboxMode)
	assert.Equal(t, "warn", cfg.Plugins.CosignPolicy)
	assert.Empty(t, cfg.Plugins.CosignPublicKey)
	assert.Equal(t, int64(256), cfg.Plugins.MemoryMB)
	assert.Equal(t, 10, cfg.Plugins.CPUNice)
	assert.Equal(t, 64, cfg.Plugins.PIDsMax)
}

func TestLoad_EnvOverrides(t *testing.T) {
	t.Setenv("EVEY_SERVER_PORT", "8080")
	t.Setenv("EVEY_DB_PORT", "5433")
	t.Setenv("EVEY_DB_MAX_OPEN_CONNS", "100")
	t.Setenv("EVEY_TRANSCODING_HW_ACCEL_ENABLED", "true")
	t.Setenv("EVEY_TRANSCODING_SINGLE_VARIANT", "0") // opt OUT of single-variant (default is single)
	t.Setenv("EVEY_TRANSCODING_MAX_SESSIONS", "2")
	t.Setenv("EVEY_OIDC_ENABLED", "true")
	t.Setenv("EVEY_JWT_ACCESS_TTL", "30m")
	t.Setenv("EVEY_LIBRARY_WATCHER_ENABLED", "false")
	t.Setenv("EVEY_LOG_LEVEL", "debug")
	t.Setenv("EVEY_REMOTE_ACCESS_ENABLED", "true")
	t.Setenv("EVEY_METADATA_LANGUAGE", "fr")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 5433, cfg.DB.Port)
	assert.Equal(t, 100, cfg.DB.MaxOpenConns)
	assert.True(t, cfg.Transcoding.HWAccelEnabled)
	assert.False(t, cfg.Transcoding.SingleVariant, "EVEY_TRANSCODING_SINGLE_VARIANT=0 opts back into the shared-decode multi-variant graph")
	assert.Equal(t, 2, cfg.Transcoding.MaxSessions)
	assert.True(t, cfg.OIDC.Enabled)
	assert.Equal(t, 30*time.Minute, cfg.JWT.AccessTokenTTL)
	assert.False(t, cfg.Library.WatcherEnabled)
	assert.Equal(t, "fr", cfg.Metadata.Language)
	assert.Equal(t, "debug", cfg.Log.Level)
	assert.True(t, cfg.RemoteAccess.Enabled)
}

func TestLoad_InvalidIntFallsBack(t *testing.T) {
	t.Setenv("EVEY_SERVER_PORT", "notanumber")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 9090, cfg.Server.Port)
}

func TestLoad_InvalidBoolFallsBack(t *testing.T) {
	t.Setenv("EVEY_OIDC_ENABLED", "notabool")

	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.OIDC.Enabled)
}

func TestLoad_InvalidDurationFallsBack(t *testing.T) {
	t.Setenv("EVEY_JWT_ACCESS_TTL", "invalid")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, 15*time.Minute, cfg.JWT.AccessTokenTTL)
}

func TestLoad_TracingEnvOverrides(t *testing.T) {
	t.Setenv("EVEY_OTLP_ENDPOINT", "otel-collector:4317")
	t.Setenv("EVEY_TRACING_SERVICE_NAME", "evey-test")
	t.Setenv("EVEY_TRACING_SAMPLE_RATIO", "0.25")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "otel-collector:4317", cfg.Tracing.OTLPEndpoint)
	assert.Equal(t, "evey-test", cfg.Tracing.ServiceName)
	assert.InDelta(t, 0.25, cfg.Tracing.SampleRatio, 0.0001)
}

func TestLoad_TracingInvalidRatioFallsBack(t *testing.T) {
	t.Setenv("EVEY_TRACING_SAMPLE_RATIO", "notafloat")

	cfg, err := Load()
	require.NoError(t, err)
	assert.InDelta(t, 0.0, cfg.Tracing.SampleRatio, 0.0001)
}

func TestDBConfig_DSN(t *testing.T) {
	t.Parallel()
	db := DBConfig{
		Host:     "db.local",
		Port:     5432,
		User:     "admin",
		Password: "secret",
		DBName:   "mydb",
		SSLMode:  "disable",
	}
	expected := "host=db.local port=5432 user=admin password=secret dbname=mydb sslmode=disable"
	assert.Equal(t, expected, db.DSN())
}

func TestDBConfig_DSN_SpecialChars(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		password string
		want     string
	}{
		{"space in password", "has space", "password='has space'"},
		{"quote in password", "quote'here", "password='quote\\'here'"},
		{"backslash in password", `back\slash`, `password='back\\slash'`},
		{"empty password", "", "password=''"},
		{"plain password", "simple", "password=simple"},
		{"at symbol ok", "p@ss", "password=p@ss"},
		{"slash ok", "p/ss", "password=p/ss"},
		{"colon ok", "p:ss", "password=p:ss"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			db := DBConfig{
				Host:     "h",
				Port:     1,
				User:     "u",
				Password: tt.password,
				DBName:   "d",
				SSLMode:  "disable",
			}
			assert.Contains(t, db.DSN(), tt.want)
		})
	}
}

// EVEY_TRUSTED_PROXIES no longer goes through envStrSlice - it has its own
// parser that adds to a mandatory baseline (see trusted_proxies.go), so the
// generic list splitting is exercised on a plain list variable instead.
func TestEnvStrSlice(t *testing.T) {
	t.Setenv("EVEY_REMOTE_ACCESS_LAN_NETWORKS", "10.0.0.1, 10.0.0.2 , 10.0.0.3")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}, cfg.RemoteAccess.LANNetworks)
}

func TestEnvStrSlice_EmptyParts(t *testing.T) {
	t.Setenv("EVEY_REMOTE_ACCESS_LAN_NETWORKS", "a,,b, ,c")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cfg.RemoteAccess.LANNetworks)
}

func TestEnvStrSlice_OIDCDomains(t *testing.T) {
	t.Setenv("EVEY_OIDC_ALLOWED_EMAIL_DOMAINS", "example.com,test.org")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"example.com", "test.org"}, cfg.OIDC.AllowedEmailDomains)
}

func TestWarnValidate_ProdDefaultDBPasswordRejected(t *testing.T) {
	cfg := &Config{
		DB:          DBConfig{Password: "evey", SSLMode: "disable", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "strong-key-abcdefghij"},
		Plugins:     PluginsConfig{SandboxMode: "basic", CosignPolicy: "off"},
	}
	err := cfg.WarnValidate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "EVEY_DB_PASSWORD")
}

// The placeholder .env.example ships is named at boot, but not refused: an
// install that copied it would otherwise stop on its next upgrade.
func TestWarnValidate_PlaceholderDBPasswordWarned(t *testing.T) {
	var logs bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logs, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })
	cfg := &Config{
		DB:          DBConfig{Password: "changeme", SSLMode: "disable", Host: "postgres"},
		Meilisearch: MeilisearchConfig{APIKey: "strong-key-abcdefghij"},
		Plugins:     PluginsConfig{SandboxMode: "basic", CosignPolicy: "off"},
	}
	require.NoError(t, cfg.WarnValidate())
	assert.Contains(t, logs.String(), "EVEY_DB_PASSWORD is the .env.example placeholder")
}

func TestWarnValidate_DevModeDowngradesToWarnings(t *testing.T) {
	cfg := &Config{
		Log:         LogConfig{DevMode: true},
		DB:          DBConfig{Password: "evey", SSLMode: "disable", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "changeme"},
		Plugins:     PluginsConfig{SandboxMode: "basic", CosignPolicy: "off"},
	}
	err := cfg.WarnValidate()
	require.NoError(t, err, "dev mode must log warnings, not return errors")
}

func TestWarnValidate_DefaultMeiliKeyRejected(t *testing.T) {
	cfg := &Config{
		DB:          DBConfig{Password: "strong", SSLMode: "disable", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "changeme"},
		Plugins:     PluginsConfig{SandboxMode: "basic", CosignPolicy: "off"},
	}
	err := cfg.WarnValidate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "MEILI_MASTER_KEY")
}

func TestWarnValidate_StrongConfigPasses(t *testing.T) {
	cfg := &Config{
		DB:          DBConfig{Password: "a-long-random-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins:     PluginsConfig{SandboxMode: "basic", CosignPolicy: "off"},
	}
	err := cfg.WarnValidate()
	require.NoError(t, err)
}

// --- PluginsConfig / SandboxMode tests ---

func TestLoad_PluginsSandboxModeDefault(t *testing.T) {
	t.Parallel()
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.Plugins.SandboxMode)
}

func TestLoad_PluginsSandboxModeEnvOverride(t *testing.T) {
	tests := []struct {
		envVal   string
		expected string
	}{
		{"basic", "basic"},
		{"bubblewrap", "bubblewrap"},
		{"strict", "strict"},
		{"landlock", "landlock"},
		{"auto", "auto"},
		// whitespace/case tolerance
		{"  STRICT  ", "strict"},
		{"Bubblewrap", "bubblewrap"},
		{"  Landlock ", "landlock"},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.envVal, func(t *testing.T) {
			t.Setenv("EVEY_PLUGINS_SANDBOX_MODE", tt.envVal)
			cfg, err := Load()
			require.NoError(t, err)
			assert.Equal(t, tt.expected, cfg.Plugins.SandboxMode)
		})
	}
}

func TestLoad_PluginsSandboxModeEmptyFallsToDefault(t *testing.T) {
	t.Setenv("EVEY_PLUGINS_SANDBOX_MODE", "")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "auto", cfg.Plugins.SandboxMode)
}

func TestWarnValidate_SandboxModeValid(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"auto", "landlock", "basic", "bubblewrap", "strict"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
				Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
				Plugins:     PluginsConfig{SandboxMode: mode, CosignPolicy: "off"},
			}
			err := cfg.WarnValidate()
			require.NoError(t, err)
		})
	}
}

func TestWarnValidate_SandboxModeInvalid(t *testing.T) {
	t.Parallel()
	for _, mode := range []string{"none", "docker", "", "unknown"} {
		mode := mode
		t.Run(mode, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
				Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
				Plugins:     PluginsConfig{SandboxMode: mode, CosignPolicy: "off"},
			}
			err := cfg.WarnValidate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cfg.Plugins.SandboxMode")
			assert.Contains(t, err.Error(), "basic|bubblewrap|strict")
		})
	}
}

func TestWarnValidate_PluginsResourceLimitsRejectNegative(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name      string
		cfg       PluginsConfig
		wantField string
	}{
		{
			name: "MemoryMB negative",
			cfg: PluginsConfig{
				SandboxMode: "basic", CosignPolicy: "off",
				MemoryMB: -1,
			},
			wantField: "cfg.Plugins.MemoryMB",
		},
		{
			name: "PIDsMax negative",
			cfg: PluginsConfig{
				SandboxMode: "basic", CosignPolicy: "off",
				PIDsMax: -100,
			},
			wantField: "cfg.Plugins.PIDsMax",
		},
		{
			name: "CPUNice out of range",
			cfg: PluginsConfig{
				SandboxMode: "basic", CosignPolicy: "off",
				CPUNice: 25,
			},
			wantField: "cfg.Plugins.CPUNice",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg := &Config{
				DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
				Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
				Plugins:     tc.cfg,
			}
			err := cfg.WarnValidate()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.wantField)
		})
	}
}

func TestWarnValidate_CosignKey_MissingFile(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "enforce",
			CosignPublicKey: "/does/not/exist.pem",
		},
	}
	err := cfg.WarnValidate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cfg.Plugins.CosignPublicKey")
	assert.Contains(t, err.Error(), "cannot read")
}

// enforce without an operator key is valid: every package carries its own
// signing key and trust comes from the trusted-publisher list, which the
// panel fills. The env key is only one way to seed that list.
func TestWarnValidate_CosignEnforce_EmptyKey_Accepted(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "enforce",
			CosignPublicKey: "",
		},
	}
	err := cfg.WarnValidate()
	if err != nil {
		assert.NotContains(t, err.Error(), "cfg.Plugins.CosignPublicKey")
	}
}

// warn without a key is tolerated (best-effort): it must NOT block startup.
func TestWarnValidate_CosignWarn_EmptyKey_Allowed(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "warn",
			CosignPublicKey: "",
		},
	}
	require.NoError(t, cfg.WarnValidate(), "warn + empty key is best-effort, must not block startup")
}

func TestWarnValidate_CosignKey_InvalidPEM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	// File exists but is not PEM-encoded.
	junk := dir + "/junk.key"
	require.NoError(t, os.WriteFile(junk, []byte("not a pem at all"), 0o600))
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "enforce",
			CosignPublicKey: junk,
		},
	}
	err := cfg.WarnValidate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a PEM")
}

func TestWarnValidate_CosignKey_WrongPEMType(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	priv := dir + "/priv.key"
	pemData := "-----BEGIN PRIVATE KEY-----\nMIIBVQIBADANBgkq\n-----END PRIVATE KEY-----\n"
	require.NoError(t, os.WriteFile(priv, []byte(pemData), 0o600))
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "enforce",
			CosignPublicKey: priv,
		},
	}
	err := cfg.WarnValidate()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "PUBLIC KEY")
}

func TestWarnValidate_CosignKey_ValidPEM(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	pub := dir + "/pub.key"
	pemData := "-----BEGIN PUBLIC KEY-----\nMFkwEwYHKoZIzj0CAQYIKoZIzj0DAQcDQgAE\n-----END PUBLIC KEY-----\n"
	require.NoError(t, os.WriteFile(pub, []byte(pemData), 0o600))
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "enforce",
			CosignPublicKey: pub,
		},
	}
	err := cfg.WarnValidate()
	require.NoError(t, err)
}

func TestWarnValidate_CosignKey_KMSRefSkipped(t *testing.T) {
	t.Parallel()
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode:     "basic",
			CosignPolicy:    "enforce",
			CosignPublicKey: "gcpkms://projects/x/locations/global/keyRings/r/cryptoKeys/k/cryptoKeyVersions/1",
		},
	}
	err := cfg.WarnValidate()
	require.NoError(t, err)
}

func TestWarnValidate_PluginsResourceLimitsZeroAccepted(t *testing.T) {
	t.Parallel()
	// Zero means "use default" - must not be rejected.
	cfg := &Config{
		DB:          DBConfig{Password: "strong-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins: PluginsConfig{
			SandboxMode: "basic", CosignPolicy: "off",
			MemoryMB: 0, PIDsMax: 0, CPUNice: 0,
		},
	}
	err := cfg.WarnValidate()
	require.NoError(t, err)
}

// Audio renditions are ON by default: they gate the hybrid mkvgo rung (video
// packaged with zero ffmpeg + audio re-encoded on the side). The A/V desync that
// once forced them off is fixed - the audio session is offset by the video's
// composition delay (has_b_frames/fps).
func TestAudioRenditionEnabledByDefault(t *testing.T) {
	t.Setenv("EVEY_DB_URL", "postgres://x/y")
	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.Transcoding.AudioRenditionEnabled,
		"audio renditions gate the hybrid mkvgo rung - on by default now the A/V offset is applied")

	t.Setenv("EVEY_HLS_AUDIO_RENDITION_ENABLED", "false")
	off, err := Load()
	require.NoError(t, err)
	assert.False(t, off.Transcoding.AudioRenditionEnabled, "operators must be able to opt out")
}

// An unparseable value used to fall back in complete silence: the operator
// read their own compose file and believed the setting had applied.
func TestLoad_UnparseableValuesAreLogged(t *testing.T) {
	cases := []struct {
		name, key, value, wantValue string
	}{
		{"int", "EVEY_SERVER_PORT", "notanumber", "notanumber"},
		{"bool", "EVEY_OIDC_ENABLED", "tru", "tru"},
		{"duration", "EVEY_JWT_ACCESS_TTL", "30", "30"},
		{"float", "EVEY_TRACING_SAMPLE_RATIO", "half", "half"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			logs := captureLogs(t)
			t.Setenv(c.key, c.value)

			_, err := Load()
			require.NoError(t, err)

			out := logs.String()
			assert.Contains(t, out, c.key, "the warning must name the variable")
			assert.Contains(t, out, c.wantValue, "the warning must quote the value received")
		})
	}
}

// A list made only of separators yields an EMPTY list, not the default - the
// opposite of what the operator typed, and previously silent.
func TestEnvStrSlice_SeparatorsOnlyWarns(t *testing.T) {
	logs := captureLogs(t)
	t.Setenv("EVEY_TEST_LIST", ", ,")

	got := envStrSlice("EVEY_TEST_LIST", []string{"a"})

	assert.Empty(t, got)
	assert.Contains(t, logs.String(), "EVEY_TEST_LIST")
}

func TestWarnValidate_PortOutOfRangeWarns(t *testing.T) {
	logs := captureLogs(t)
	cfg := strongConfig()
	cfg.Server.Port = 70000

	require.NoError(t, cfg.WarnValidate())
	assert.Contains(t, logs.String(), "EVEY_SERVER_PORT")
}

// database/sql caps idle at open without a word, handing back a smaller pool
// than the one configured.
func TestWarnValidate_IdleAboveOpenWarns(t *testing.T) {
	logs := captureLogs(t)
	cfg := strongConfig()
	cfg.DB.MaxOpenConns = 5
	cfg.DB.MaxIdleConns = 25

	require.NoError(t, cfg.WarnValidate())
	assert.Contains(t, logs.String(), "EVEY_DB_MAX_IDLE_CONNS")
}

// "50" meaning 50% samples every single trace.
func TestWarnValidate_SampleRatioOutOfRangeWarns(t *testing.T) {
	logs := captureLogs(t)
	cfg := strongConfig()
	cfg.Tracing.SampleRatio = 50

	require.NoError(t, cfg.WarnValidate())
	assert.Contains(t, logs.String(), "EVEY_TRACING_SAMPLE_RATIO")
}

// The sane baseline must stay quiet, or every operator learns to ignore these.
func TestWarnValidate_SaneConfigWarnsAboutNone(t *testing.T) {
	logs := captureLogs(t)
	cfg := strongConfig()
	cfg.Server.Port = 9090
	cfg.DB.Port = 5432
	cfg.SMTP.Port = 587
	cfg.DB.MaxOpenConns = 25
	cfg.DB.MaxIdleConns = 10
	cfg.Tracing.SampleRatio = 0.1

	require.NoError(t, cfg.WarnValidate())

	out := logs.String()
	for _, key := range []string{"EVEY_SERVER_PORT", "EVEY_DB_PORT", "EVEY_SMTP_PORT", "EVEY_DB_MAX_IDLE_CONNS", "EVEY_TRACING_SAMPLE_RATIO"} {
		assert.NotContains(t, out, key)
	}
}

// strongConfig is the baseline the range checks are measured against: it
// passes WarnValidate outright, so any warning a test sees comes from the one
// field it deliberately broke.
func strongConfig() *Config {
	return &Config{
		DB:          DBConfig{Password: "a-long-random-password-xyz", SSLMode: "require", Host: "localhost"},
		Meilisearch: MeilisearchConfig{APIKey: "x9abc123def45678"},
		Plugins:     PluginsConfig{SandboxMode: "basic", CosignPolicy: "off"},
	}
}

// The image ships EVEY_DB_PASSWORD_FILE / MEILI_MASTER_KEY_FILE baked in so the
// secrets the entrypoint generates on first boot reach `docker exec evey admin`
// too. The precedence is what keeps existing installs untouched: an explicit
// variable must always beat the file.
func TestEnvSecret_FileFallbackAndPrecedence(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/postgres.password"
	require.NoError(t, os.WriteFile(path, []byte("from-file\n"), 0o600))

	t.Setenv("EVEY_TEST_SECRET", "")
	os.Unsetenv("EVEY_TEST_SECRET")
	t.Setenv("EVEY_TEST_SECRET_FILE", path)
	assert.Equal(t, "from-file", envSecret("EVEY_TEST_SECRET", "dflt"), "file wins over the default and is trimmed")

	t.Setenv("EVEY_TEST_SECRET", "from-env")
	assert.Equal(t, "from-env", envSecret("EVEY_TEST_SECRET", "dflt"), "explicit variable beats the file")

	os.Unsetenv("EVEY_TEST_SECRET")
	t.Setenv("EVEY_TEST_SECRET_FILE", dir+"/missing")
	assert.Equal(t, "dflt", envSecret("EVEY_TEST_SECRET", "dflt"), "a missing file is an unset secret")

	require.NoError(t, os.WriteFile(path, []byte("  \n"), 0o600))
	t.Setenv("EVEY_TEST_SECRET_FILE", path)
	assert.Equal(t, "dflt", envSecret("EVEY_TEST_SECRET", "dflt"), "a blank file is an unset secret")
}

func TestLoad_SecretsFromFile(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(dir+"/pg", []byte("pg-secret"), 0o600))
	require.NoError(t, os.WriteFile(dir+"/meili", []byte("meili-secret"), 0o600))
	t.Setenv("EVEY_DB_PASSWORD", "")
	os.Unsetenv("EVEY_DB_PASSWORD")
	t.Setenv("MEILI_MASTER_KEY", "")
	os.Unsetenv("MEILI_MASTER_KEY")
	t.Setenv("EVEY_DB_PASSWORD_FILE", dir+"/pg")
	t.Setenv("MEILI_MASTER_KEY_FILE", dir+"/meili")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "pg-secret", cfg.DB.Password)
	assert.Equal(t, "meili-secret", cfg.Meilisearch.APIKey)
}

func TestLoad_SSLModeDefaultDependsOnHost(t *testing.T) {
	t.Setenv("EVEY_DB_SSLMODE", "")
	os.Unsetenv("EVEY_DB_SSLMODE")
	t.Setenv("EVEY_DB_HOST", "postgres")
	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, "prefer", cfg.DB.SSLMode, "a remote host keeps the historical default")

	t.Setenv("EVEY_DB_HOST", "127.0.0.1")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, "disable", cfg.DB.SSLMode)

	t.Setenv("EVEY_DB_SSLMODE", "require")
	cfg, err = Load()
	require.NoError(t, err)
	assert.Equal(t, "require", cfg.DB.SSLMode, "an explicit mode always wins")
}

func TestLoad_ForwardedProtoFollowsBaseURLScheme(t *testing.T) {
	t.Setenv("EVEY_FORWARDED_PROTO", "")
	os.Unsetenv("EVEY_FORWARDED_PROTO")

	t.Setenv("EVEY_SERVER_BASE_URL", "https://media.example.com")
	cfg, err := Load()
	require.NoError(t, err)
	assert.True(t, cfg.Server.ForwardedProto, "an https base URL means a proxy terminates TLS in front")

	t.Setenv("EVEY_SERVER_BASE_URL", "HTTPS://media.example.com/")
	cfg, err = Load()
	require.NoError(t, err)
	assert.True(t, cfg.Server.ForwardedProto, "scheme match is case-insensitive")

	t.Setenv("EVEY_SERVER_BASE_URL", "http://nas.local:9091")
	cfg, err = Load()
	require.NoError(t, err)
	assert.False(t, cfg.Server.ForwardedProto)
}

func TestLoad_ForwardedProtoExplicitOverridesBaseURL(t *testing.T) {
	buf := captureLogs(t)

	t.Setenv("EVEY_SERVER_BASE_URL", "https://media.example.com")
	t.Setenv("EVEY_FORWARDED_PROTO", "false")
	cfg, err := Load()
	require.NoError(t, err)
	assert.False(t, cfg.Server.ForwardedProto, "an explicit value still wins")
	assert.Contains(t, buf.String(), "EVEY_FORWARDED_PROTO is deprecated")

	t.Setenv("EVEY_SERVER_BASE_URL", "http://nas.local:9091")
	t.Setenv("EVEY_FORWARDED_PROTO", "true")
	cfg, err = Load()
	require.NoError(t, err)
	assert.True(t, cfg.Server.ForwardedProto)
}
