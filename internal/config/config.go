package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

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
	SMTP         SMTPConfig
	FCM          FCMConfig
}

type FCMConfig struct {
	Enabled            bool   // FCM_ENABLED
	ServiceAccountJSON string // FCM_SERVICE_ACCOUNT_JSON — path to service account JSON file
	ProjectID          string // FCM_PROJECT_ID
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

type MetadataConfig struct {
	Language string // ISO 639-1, default "en"
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
	TrustedProxies    []string
	RealIPHeader      string
	ForwardedProto    bool
	LoginRateLimitMax int
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
	return fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=%s",
		c.Host, c.Port, c.User, c.Password, c.DBName, c.SSLMode,
	)
}

type MeilisearchConfig struct {
	URL    string
	APIKey string
}

type JWTConfig struct {
	PrivateKeyPath  string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
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
	DefaultScanInterval time.Duration
	WatcherEnabled      bool
}

type PlaybackConfig struct {
	DefaultMaxStreams   int
	SkipForwardSeconds  int
	SkipBackwardSeconds int
}

// WarnValidate logs warnings for insecure or suspicious config values.
// It never returns an error — the app should still start.
func (c *Config) WarnValidate() {
	// OPS-NEW-004: Meilisearch master key
	k := c.Meilisearch.APIKey
	if k == "" || k == "changeme" || k == "change-me-generate-a-random-secret" || len(k) < 16 {
		slog.Warn("meilisearch API key is weak or default — change EVEY_MEILI_API_KEY",
			slog.String("current", k))
	}

	// OPS-NEW-001: FCM service account JSON
	if c.FCM.Enabled && c.FCM.ServiceAccountJSON != "" {
		if _, err := os.Stat(c.FCM.ServiceAccountJSON); err != nil {
			slog.Warn("FCM service account file not accessible",
				slog.String("path", c.FCM.ServiceAccountJSON),
				slog.Any("err", err))
		}
	}

	// OPS-NEW-002: OIDC client secret
	if c.OIDC.Enabled && c.OIDC.Issuer != "" && c.OIDC.ClientSecret == "" {
		slog.Warn("OIDC enabled but client secret is empty — set EVEY_OIDC_CLIENT_SECRET")
	}
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:              envInt("EVEY_SERVER_PORT", 9090),
			BaseURL:           envStr("EVEY_SERVER_BASE_URL", "http://localhost:9091"),
			Name:              envStr("EVEY_SERVER_NAME", "Evey"),
			TrustedProxies:    envStrSlice("EVEY_TRUSTED_PROXIES", []string{"127.0.0.1/32", "::1/128"}),
			RealIPHeader:      envStr("EVEY_REAL_IP_HEADER", "X-Forwarded-For"),
			ForwardedProto:    envBool("EVEY_FORWARDED_PROTO", false),
			LoginRateLimitMax: envInt("EVEY_RATELIMIT_LOGIN_MAX", 10),
		},
		DB: DBConfig{
			Host:         envStr("EVEY_DB_HOST", "localhost"),
			Port:         envInt("EVEY_DB_PORT", 5432),
			User:         envStr("EVEY_DB_USER", "evey"),
			Password:     envStr("EVEY_DB_PASSWORD", "evey"),
			DBName:       envStr("EVEY_DB_NAME", "evey"),
			SSLMode:      envStr("EVEY_DB_SSLMODE", "prefer"),
			MaxOpenConns: envInt("EVEY_DB_MAX_OPEN_CONNS", 50),
			MaxIdleConns: envInt("EVEY_DB_MAX_IDLE_CONNS", 10),
		},
		Meilisearch: MeilisearchConfig{
			URL:    envStr("EVEY_MEILI_URL", "http://localhost:7700"),
			APIKey: envStr("EVEY_MEILI_API_KEY", "changeme"),
		},
		JWT: JWTConfig{
			PrivateKeyPath:  envStr("EVEY_JWT_PRIVATE_KEY_PATH", "/data/jwt.key"),
			AccessTokenTTL:  envDuration("EVEY_JWT_ACCESS_TTL", 15*time.Minute),
			RefreshTokenTTL: envDuration("EVEY_JWT_REFRESH_TTL", 7*24*time.Hour),
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
			DefaultScanInterval: envDuration("EVEY_LIBRARY_SCAN_INTERVAL", 6*time.Hour),
			WatcherEnabled:      envBool("EVEY_LIBRARY_WATCHER_ENABLED", true),
		},
		Playback: PlaybackConfig{
			DefaultMaxStreams:   envInt("EVEY_PLAYBACK_MAX_STREAMS", 0),
			SkipForwardSeconds:  envInt("EVEY_PLAYBACK_SKIP_FORWARD", 10),
			SkipBackwardSeconds: envInt("EVEY_PLAYBACK_SKIP_BACKWARD", 10),
		},
		Metadata: MetadataConfig{
			Language: envStr("EVEY_METADATA_LANGUAGE", "en"),
		},
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
		FCM: FCMConfig{
			Enabled:            envBool("FCM_ENABLED", false),
			ServiceAccountJSON: envStr("FCM_SERVICE_ACCOUNT_JSON", ""),
			ProjectID:          envStr("FCM_PROJECT_ID", ""),
		},
	}
	return cfg, nil
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
