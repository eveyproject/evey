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
	// DumpAll streams a whole-database, data-only dump prefixed with a TRUNCATE of
	// every table, so a restore replaces the data on a populated base. When
	// includeTelemetry is false the high-volume telemetry tables are emptied but
	// not reloaded (excluded from the dump), keeping routine backups lean. The
	// returned ReadCloser MUST be closed by the caller, even on an error path that
	// abandons it unread: the dump is backed by an io.Pipe whose writer goroutine
	// blocks on Write until a reader drains it, so dropping it (e.g. a temp-file
	// error before streaming) would wedge that goroutine and leave the pg_dump
	// child unreaped. Close unblocks it.
	DumpAll(ctx context.Context, includeTelemetry bool) (io.ReadCloser, error)
	RestoreTables(ctx context.Context, data io.Reader) error
	// CurrentSchemaVersion reads the current schema version from schema_migrations.
	// Returns an error if the schema is in a dirty state (failed migration).
	CurrentSchemaVersion(ctx context.Context) (string, error)
}

// ScheduleRepository is the port for the singleton backup_schedule row.
//
// The run bookkeeping is split in two on purpose. AdvanceNextRun fires at
// TRIGGER time so a backup that outlives the tick interval cannot be re-fired
// by the next tick (the scheduler used to advance only after CreateBackup
// returned, stacking one pg_dump goroutine per minute behind a slow run).
// RecordRunResult lands after the run with its outcome. Standard cron
// semantics, same as the library scan scheduler.
type ScheduleRepository interface {
	Get(ctx context.Context) (*BackupSchedule, error)
	Update(ctx context.Context, s *BackupSchedule) error
	AdvanceNextRun(ctx context.Context, nextRun time.Time) error
	RecordRunResult(ctx context.Context, lastRun time.Time, status, errMsg string) error
}
