package backup

import (
	"errors"
	"slices"
	"time"
)

// ErrNotFound is returned by the BackupStore when the requested backup does not
// exist. Callers can distinguish a missing backup from a real store failure with
// errors.Is.
var ErrNotFound = errors.New("backup not found")

// BackupSchedule is the singleton row that drives the cron-based backup scheduler.
type BackupSchedule struct {
	CronExpression string
	Enabled        bool
	RetentionDays  int
	LastRunAt      *time.Time
	NextRunAt      *time.Time
	LastStatus     string // "success" | "failed"
	LastError      string
	UpdatedAt      time.Time
}

type BackupManifest struct {
	ID            string    `json:"id"`
	CreatedAt     time.Time `json:"created_at"`
	Version       string    `json:"version"`
	SchemaVersion string    `json:"schema_version,omitempty"`
	Tables        []string  `json:"tables"`
	Size          int64     `json:"size"`
	DumpSHA256    string    `json:"dump_sha256,omitempty"`
}

func (m *BackupManifest) Validate() error {
	if m.ID == "" {
		return errors.New("backup manifest requires an ID")
	}
	if m.Version == "" {
		return errors.New("backup manifest requires a version")
	}
	if len(m.Tables) == 0 {
		return errors.New("backup manifest requires at least one table")
	}
	return nil
}

type BackupManifestOption func(*BackupManifest)

func WithBackupSize(size int64) BackupManifestOption {
	return func(m *BackupManifest) { m.Size = size }
}

func WithBackupSHA256(sha string) BackupManifestOption {
	return func(m *BackupManifest) { m.DumpSHA256 = sha }
}

// WithBackupSchemaVersion stamps the schema_migrations version at the time
// the backup was created. Used at restore time to guard against restoring a
// dump into a schema that has since been migrated forward.
func WithBackupSchemaVersion(v string) BackupManifestOption {
	return func(m *BackupManifest) { m.SchemaVersion = v }
}

// NewBackupManifest is the canonical constructor. Stamps CreatedAt
// (UTC) and validates. Required fields (id, version, tables) are
// surfaced as params so construction never succeeds with them missing.
func NewBackupManifest(id, version string, tables []string, opts ...BackupManifestOption) (*BackupManifest, error) {
	m := &BackupManifest{
		ID:        id,
		Version:   version,
		Tables:    slices.Clone(tables),
		CreatedAt: time.Now().UTC(),
	}
	for _, opt := range opts {
		opt(m)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return m, nil
}
