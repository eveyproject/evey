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
	ActiveStreams      int
	ActiveTranscodes  int
	BandwidthBps      int64
	StorageUsedBytes  int64
	StorageTotalBytes int64
	RecordedAt        time.Time
}

type UserStats struct {
	ProfileID        string
	TotalWatchTimeMs int64
	TotalPlayCount   int
	GenreBreakdown   map[string]int64
	MostPlayedItems  []MediaPlayCount
	PlayCountByMonth map[string]int
}

type MediaPlayCount struct {
	MediaItemID string
	Title       string
	PlayCount   int
}

type AuditLogEntry struct {
	ID           string
	UserID       string
	Action       string
	ResourceType string
	ResourceID   *string
	Details      map[string]interface{}
	ClientIP     *string
	UserAgent    *string
	DeviceName   *string
	CreatedAt    time.Time
}

type ServerStatsResult struct {
	ActiveStreams     int
	ActiveTranscodes int
	TotalUsers       int
	TotalItems       int
	TotalPlayTimeMs  int64
}
