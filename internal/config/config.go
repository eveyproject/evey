package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Server      ServerConfig
	DB          DBConfig
	Meilisearch MeilisearchConfig
	JWT         JWTConfig
	Transcoding TranscodingConfig
	OIDC        OIDCConfig
	Metrics     MetricsConfig
	Library     LibraryConfig
	Playback    PlaybackConfig
	Metadata    MetadataConfig
	Log         LogConfig
}

type MetadataConfig struct {
	Language string // ISO 639-1, default "en"
}

type LogConfig struct {
	Level         string
	Dir           string
	MaxSizeMB     int
	RetentionDays int
	DevMode       bool
	PluginDebug   bool
}

type ServerConfig struct {
	Port    int
	BaseURL string
	Name    string
}

type DBConfig struct {
	Host     string
	Port     int
	User     string
	Password string
	DBName   string
	SSLMode  string
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
	MaxConcurrent  int
	CacheMaxGB     int
	CacheDir       string
	HWAccelEnabled bool
}

type OIDCConfig struct {
	Enabled      bool
	Issuer       string
	ClientID     string
	ClientSecret string
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
	DefaultMaxStreams int
}

func Load() (*Config, error) {
	cfg := &Config{
		Server: ServerConfig{
			Port:    envInt("EVEY_SERVER_PORT", 9090),
			BaseURL: envStr("EVEY_SERVER_BASE_URL", "http://localhost:9091"),
			Name:    envStr("EVEY_SERVER_NAME", "Evey"),
		},
		DB: DBConfig{
			Host:     envStr("EVEY_DB_HOST", "localhost"),
			Port:     envInt("EVEY_DB_PORT", 5432),
			User:     envStr("EVEY_DB_USER", "evey"),
			Password: envStr("EVEY_DB_PASSWORD", "evey"),
			DBName:   envStr("EVEY_DB_NAME", "evey"),
			SSLMode:  envStr("EVEY_DB_SSLMODE", "prefer"),
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
			MaxConcurrent:  envInt("EVEY_TRANSCODING_MAX_CONCURRENT", 2),
			CacheMaxGB:     envInt("EVEY_TRANSCODING_CACHE_MAX_GB", 50),
			CacheDir:       envStr("EVEY_TRANSCODING_CACHE_DIR", "/data/transcode"),
			HWAccelEnabled: envBool("EVEY_TRANSCODING_HW_ACCEL_ENABLED", false),
		},
		OIDC: OIDCConfig{
			Enabled:      envBool("EVEY_OIDC_ENABLED", false),
			Issuer:       envStr("EVEY_OIDC_ISSUER", ""),
			ClientID:     envStr("EVEY_OIDC_CLIENT_ID", ""),
			ClientSecret: envStr("EVEY_OIDC_CLIENT_SECRET", ""),
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
			DefaultMaxStreams: envInt("EVEY_PLAYBACK_MAX_STREAMS", 0),
		},
		Metadata: MetadataConfig{
			Language: envStr("EVEY_METADATA_LANGUAGE", "en"),
		},
		Log: LogConfig{
			Level:         envStr("EVEY_LOG_LEVEL", "info"),
			Dir:           envStr("EVEY_LOG_DIR", "logs"),
			MaxSizeMB:     envInt("EVEY_LOG_MAX_SIZE_MB", 50),
			RetentionDays: envInt("EVEY_LOG_RETENTION_DAYS", 7),
			DevMode:       envBool("EVEY_LOG_DEV_MODE", false),
			PluginDebug:   envBool("EVEY_PLUGIN_DEBUG", false),
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

func envDuration(key string, fallback time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return fallback
}
