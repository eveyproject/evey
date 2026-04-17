package config

import (
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
	assert.Equal(t, 10, cfg.DB.MaxIdleConns)

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

	assert.Equal(t, 6*time.Hour, cfg.Library.DefaultScanInterval)
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
