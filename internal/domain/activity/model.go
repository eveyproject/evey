package activity

import "time"

type PlayHistoryEntry struct {
	ID          string
	ProfileID   string
	MediaItemID string
	MediaTitle  string
	MediaType   string
	DurationMs  int64
	PositionMs  int64
	Completed   bool
	DeviceName  string
	ClientName  string
	PlayMethod  string
	StartedAt   time.Time
	EndedAt     *time.Time
}

type AuthMethod string

const (
	Local        AuthMethod = "local"
	OIDC         AuthMethod = "oidc"
	APIKeyAuth   AuthMethod = "api_key"
	QuickConnect AuthMethod = "quick_connect"
)

type LoginEntry struct {
	ID            string
	UserID        string
	ClientIP      string
	UserAgent     string
	AuthMethod    string
	Success       bool
	FailureReason *string
	CreatedAt     time.Time
}

type ServerMetric struct {
	ID                string
	CPUPercent        float64
	MemoryUsedBytes   int64
	MemoryTotalBytes  int64
	ActiveStreams     int
	ActiveTranscodes  int
	BandwidthBps      int64
	StorageUsedBytes  int64
	StorageTotalBytes int64
	RecordedAt        time.Time
}

type UserStats struct {
	ProfileID          string
	TotalWatchTimeMs   int64
	TotalPlayCount     int
	UniqueItemsWatched int
	GenreBreakdown     map[string]GenreStats
	MostPlayedItems    []MediaPlayCount
	PlayCountByMonth   map[string]int
}

type GenreStats struct {
	TotalTimeMs int64
	PlayCount   int
}

type MediaPlayCount struct {
	MediaItemID  string
	Title        string
	PlayCount    int
	TotalTimeMs  int64
}

type AuditLogEntry struct {
	ID           string
	UserID       string
	Username     string // populated by JOIN, not persisted
	Action       string
	ResourceType string
	ResourceID   *string
	ResourceName string // resolved by JOIN, not persisted
	Details      map[string]any
	ClientIP     *string
	UserAgent    *string
	DeviceName   *string
	CreatedAt    time.Time
}

type ServerStatsResult struct {
	ActiveStreams    int
	ActiveTranscodes int
	TotalUsers       int
	TotalItems       int
	TotalPlayTimeMs  int64
}

type TopMediaItem struct {
	MediaItemID          string
	Title                string
	PlayCount            int
	TotalDurationSeconds int64
}

type RecentActivityEntry struct {
	UserID     string
	Username   string
	MediaTitle string
	PlayedAt   time.Time
}

type DailyPlayCount struct {
	Date               string
	PlayCount          int
	WatchTimeSeconds   int64
}

type PlayMethodCount struct {
	Method string
	Count  int
}

type HourlyPlayCount struct {
	Hour  int
	Count int
}

type AnalyticsData struct {
	TopMedia            []TopMediaItem
	RecentActivity      []RecentActivityEntry
	TotalPlaysToday     int
	TotalPlaysThisWeek  int
	ActiveUsersToday    int
	DailyPlays          []DailyPlayCount
	PlayMethods         []PlayMethodCount
	HourlyPlays         []HourlyPlayCount
}
