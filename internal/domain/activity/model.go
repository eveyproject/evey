package activity

import (
	"time"

	"github.com/evey-project/evey/internal/domain/shared"
)

type PlayHistoryEntry struct {
	ID          string
	ProfileID   shared.ProfileID
	MediaItemID shared.MediaItemID
	MediaTitle  string
	MediaType   string
	Duration    shared.Duration
	Position    shared.Duration
	Completed   bool
	DeviceName  string
	ClientName  string
	PlayMethod  string
	StartedAt   time.Time
	EndedAt     *time.Time
	// Who watched, populated by JOIN and never persisted on the session row.
	// Empty when the profile (or its owner) has since been deleted.
	ProfileName string
	UserID      shared.UserID
	Username    string
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
	UserID        shared.UserID
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
	ProfileID          shared.ProfileID
	TotalWatchTimeMs   int64
	TotalPlayCount     int
	UniqueItemsWatched int
	GenreBreakdown     map[string]GenreStats
	MostPlayedItems    []MediaPlayCount
	PlayByMonth        map[string]MonthlyStats
}

type MonthlyStats struct {
	PlayCount   int
	TotalTimeMs int64
}

type GenreStats struct {
	TotalTimeMs int64
	PlayCount   int
}

type MediaPlayCount struct {
	MediaItemID shared.MediaItemID
	Title       string
	PlayCount   int
	TotalTimeMs int64
}

type AuditLogEntry struct {
	ID       string
	UserID   shared.UserID
	Username string // populated by JOIN, not persisted
	// ProfileID is persisted with the entry: a profile can be renamed or
	// deleted, and the trace must survive it. ProfileName comes from a JOIN
	// and is empty once the profile is gone.
	ProfileID    shared.ProfileID
	ProfileName  string
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
	// TotalPlays counts every play session ever started.
	TotalPlays int
	// TotalStorageBytes sums the on-disk size of every media file.
	TotalStorageBytes int64
	// TotalBandwidthBytes is filled by the query service from the metric
	// history (integral of the sampled streaming rate) - the stats reader
	// itself leaves it 0.
	TotalBandwidthBytes int64
	PopularMedia        []ServerPopularMedia
	ActiveUsers         []ServerActiveUser
}

// ServerPopularMedia is one row of the admin "most played" rollup (90 days).
type ServerPopularMedia struct {
	MediaItemID   string
	Title         string
	Type          string
	PlayCount     int
	UniqueViewers int
}

// ServerActiveUser is one row of the admin "most active users" rollup (30 days).
type ServerActiveUser struct {
	UserID           string
	Username         string
	PlayCount        int
	TotalWatchTimeMs int64
	LastActiveAt     time.Time
}

type TopMediaItem struct {
	MediaItemID          shared.MediaItemID
	Title                string
	PlayCount            int
	TotalDurationSeconds int64
}

type RecentActivityEntry struct {
	UserID     shared.UserID
	Username   string
	MediaTitle string
	PlayedAt   time.Time
}

type DailyPlayCount struct {
	Date             string
	PlayCount        int
	WatchTimeSeconds int64
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
	TopMedia           []TopMediaItem
	RecentActivity     []RecentActivityEntry
	TotalPlaysToday    int
	TotalPlaysThisWeek int
	ActiveUsersToday   int
	DailyPlays         []DailyPlayCount
	PlayMethods        []PlayMethodCount
	HourlyPlays        []HourlyPlayCount
}
