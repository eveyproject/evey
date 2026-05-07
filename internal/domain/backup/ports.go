package backup

import (
	"context"
	"io"
	"time"
)

type BackupStore interface {
	Create(ctx context.Context, manifest *BackupManifest, data io.Reader) error
	List(ctx context.Context) ([]*BackupManifest, error)
	Get(ctx context.Context, id string) (*BackupManifest, io.ReadCloser, error)
	Delete(ctx context.Context, id string) error
}

type DatabaseDumper interface {
	DumpTables(ctx context.Context, tables []string) (io.Reader, error)
	RestoreTables(ctx context.Context, data io.Reader) error
}

// ScheduleRepository is the port for the singleton backup_schedule row.
type ScheduleRepository interface {
	Get(ctx context.Context) (*BackupSchedule, error)
	Update(ctx context.Context, s *BackupSchedule) error
	UpdateAfterRun(ctx context.Context, lastRun, nextRun time.Time, status, errMsg string) error
}
