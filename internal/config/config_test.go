package config

import (
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad_Defaults(t *testing.T) {
	t.Parallel()
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
	assert.Equal(t, "prefer", cfg.DB.SSLMode)
	assert.Equal(t, 50, cfg.DB.MaxOpenConns)
	assert.Equal(t, 20, cfg.DB.MaxIdleConns)

	assert.Equal(t, "http://localhost:7700", cfg.Meilisearch.URL)
	assert.Equal(t, "changeme", cfg.Meilisearch.APIKey)

	assert.Equal(t, "/data/jwt.key", cfg.JWT.PrivateKeyPath)
	assert.Equal(t, 15*time.Minute, cfg.JWT.AccessTokenTTL)
	assert.Equal(t, 7*24*time.Hour, cfg.JWT.RefreshTokenTTL)

	assert.Equal(t, 2, cfg.Transcoding.MaxConcurrent)
	assert.Equal(t, 50, cfg.Transcoding.CacheMaxGB)
	assert.Equal(t, "/data/transcode", cfg.Transcoding.CacheDir)
	assert.False(t, cfg.Transcoding.HWAccelEnabled)
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

	assert.Equal(t, 0, cfg.Playback.DefaultMaxStreams)
	assert.Equal(t, 10, cfg.Playback.SkipForwardSeconds)
	assert.Equal(t, 10, cfg.Playback.SkipBackwardSeconds)

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
	assert.False(t, cfg.Log.PluginDebug)

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
	// "off" default — only admins who configure a key get verification.
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
	t.Setenv("EVEY_OIDC_ENABLED", "true")
	t.Setenv("EVEY_JWT_ACCESS_TTL", "30m")
	t.Setenv("EVEY_LIBRARY_WATCHER_ENABLED", "false")
	t.Setenv("EVEY_LOG_LEVEL", "debug")
	t.Setenv("EVEY_REMOTE_ACCESS_ENABLED", "true")

	cfg, err := Load()
	require.NoError(t, err)

	assert.Equal(t, 8080, cfg.Server.Port)
	assert.Equal(t, 5433, cfg.DB.Port)
	assert.Equal(t, 100, cfg.DB.MaxOpenConns)
	assert.True(t, cfg.Transcoding.HWAccelEnabled)
	assert.True(t, cfg.OIDC.Enabled)
	assert.Equal(t, 30*time.Minute, cfg.JWT.AccessTokenTTL)
	assert.False(t, cfg.Library.WatcherEnabled)
	assert.Equal(t, "en", cfg.Metadata.Language)
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

func TestEnvStrSlice(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", "10.0.0.1, 10.0.0.2 , 10.0.0.3")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"10.0.0.1", "10.0.0.2", "10.0.0.3"}, cfg.Server.TrustedProxies)
}

func TestEnvStrSlice_EmptyParts(t *testing.T) {
	t.Setenv("EVEY_TRUSTED_PROXIES", "a,,b, ,c")

	cfg, err := Load()
	require.NoError(t, err)
	assert.Equal(t, []string{"a", "b", "c"}, cfg.Server.TrustedProxies)
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
	// Zero means "use default" — must not be rejected.
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
