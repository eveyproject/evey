package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"github.com/evey-project/evey/internal/domain/backup"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/id"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// ErrInvalidBackup is returned when the uploaded archive is malformed, tampered,
// or fails integrity validation. Callers can distinguish this from genuine
// internal/IO failures with errors.Is.
var ErrInvalidBackup = errors.New("backup rejected: invalid or tampered archive")

// maxRestoreDumpBytes bounds the decompressed dump.sql during restore - a
// gzip-bomb guard (G110). Package-level var so tests can lower it without
// materialising a multi-GiB fixture.
var maxRestoreDumpBytes int64 = 8 << 30 // 8 GiB

var BackupTables = []string{
	"users",
	"profiles",
	"libraries",
	// Media catalogue (films/series + metadata). Technically re-scannable from
	// disk, but included so a restore is complete AND watch_progress (keyed by
	// media_item_id) keeps valid references instead of dangling until a rescan
	// regenerates the rows - matching how music_tracks/photos are already backed
	// up. Restore order is irrelevant: pg_dump runs with --disable-triggers.
	"people",
	"media_items",
	"media_files",
	"media_catalog",
	"media_classifications",
	"media_streams",
	"chapters",
	"media_segments",
	"media_trailers",
	"media_people",
	"media_item_translations",
	"person_translations",
	"roles",
	"user_roles",
	"watch_progress",
	"play_sessions",
	"collections",
	"collection_items",
	"plugins",
	"server_config",
	"scheduled_tasks",
	"notifications",
	"notification_settings",
	"feed_admin_config",
	"feed_profile_config",
	"webhook_configs",
	"webhook_deliveries",
	"favorites",
	"playlists",
	"playlist_items",
	"scan_schedules",
	"library_access",
	"audit_log",
	"login_history",
	"music_tracks",
	"photos",
	"push_subscriptions",
	"api_keys",
}

type Service struct {
	store            backup.BackupStore
	dumper           backup.DatabaseDumper
	version          string
	events           shared.EventBus
	includeTelemetry func() bool
}

type Option func(*Service)

// WithIncludeTelemetry injects a live reader for the "include telemetry in
// backups" setting (default: exclude, keeping routine backups lean). Read per
// run by both manual and scheduled backups, so a panel or CLI change applies to
// the next backup.
func WithIncludeTelemetry(f func() bool) Option {
	return func(s *Service) { s.includeTelemetry = f }
}

func WithEventBus(bus shared.EventBus) Option {
	return func(s *Service) { s.events = bus }
}

func NewService(store backup.BackupStore, dumper backup.DatabaseDumper, version string, opts ...Option) *Service {
	s := &Service{store: store, dumper: dumper, version: version}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// CreateBackup dumps, stores and announces a backup: backup.created on
// success, backup.failed otherwise, whoever asked for it (admin or schedule).
func (s *Service) CreateBackup(ctx context.Context) (*backup.BackupManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.CreateBackup")
	defer span.End()
	m, err := s.createBackup(ctx)
	if s.events == nil {
		return m, err
	}
	var evt shared.Event
	if err != nil {
		evt = backup.BackupFailed{Error: err.Error(), OccurredAt: time.Now()}
	} else {
		evt = backup.BackupCreated{BackupID: m.ID, SizeBytes: m.Size, OccurredAt: time.Now()}
	}
	if pubErr := s.events.Publish(ctx, evt); pubErr != nil {
		slog.WarnContext(ctx, "publish backup event", slog.String("event", evt.EventName()), slog.Any("err", pubErr))
	}
	return m, err
}

func (s *Service) createBackup(ctx context.Context) (*backup.BackupManifest, error) {

	schemaVersion, err := s.dumper.CurrentSchemaVersion(ctx)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: check schema version: %w", err)
	}

	includeTelemetry := false
	if s.includeTelemetry != nil {
		includeTelemetry = s.includeTelemetry()
	}
	data, err := s.dumper.DumpAll(ctx, includeTelemetry)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: %w", err)
	}
	// MUST close on every exit path: the dump's writer goroutine blocks on Write
	// until drained, so an early return below (temp-file or copy error) that
	// abandons `data` unread would wedge that goroutine + leak the pg_dump child.
	defer data.Close()

	tmp, err := os.CreateTemp("", "evey-backup-*.sql")
	if err != nil {
		return nil, fmt.Errorf("create temp file: %w", err)
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // best-effort cleanup
	defer tmp.Close()

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmp, h), data); err != nil {
		return nil, fmt.Errorf("stream dump to temp: %w", err)
	}
	if _, err := tmp.Seek(0, io.SeekStart); err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: %w", err)
	}

	m, err := backup.NewBackupManifest(id.New("bk-"), s.version, BackupTables,
		backup.WithBackupSHA256(hex.EncodeToString(h.Sum(nil))),
		backup.WithBackupSchemaVersion(schemaVersion),
	)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: %w", err)
	}

	if err := s.store.Create(ctx, m, tmp); err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: %w", err)
	}
	return m, nil
}

func (s *Service) ListBackups(ctx context.Context) ([]*backup.BackupManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.ListBackups")
	defer span.End()

	return s.store.List(ctx)
}

func (s *Service) GetBackup(ctx context.Context, backupID string) (*backup.BackupManifest, io.ReadCloser, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.GetBackup")
	defer span.End()

	return s.store.Get(ctx, backupID)
}

func (s *Service) RestoreBackup(ctx context.Context, data io.Reader) error {
	ctx, span := tracing.StartSpan(ctx, "app.backup.RestoreBackup")
	defer span.End()

	// Obtain the current schema version before extracting the archive so that
	// a dirty schema (failed migration) is caught immediately, before we spend
	// time decompressing a potentially large dump.
	currentSchemaVersion, err := s.dumper.CurrentSchemaVersion(ctx)
	if err != nil {
		return fmt.Errorf("restore: check current schema version: %w", err)
	}

	archive, extractErr := extractTarGz(data)
	if extractErr != nil {
		// extractTarGz already wraps bad-input errors with ErrInvalidBackup;
		// IO errors (temp-file creation) are plain and propagate as-is.
		return fmt.Errorf("restore: %w", extractErr)
	}
	defer os.Remove(archive.dumpPath) //nolint:errcheck

	if archive.manifest.DumpSHA256 == "" {
		return fmt.Errorf("restore: missing dump checksum in manifest: %w", ErrInvalidBackup)
	}
	if archive.dumpSHA256 != archive.manifest.DumpSHA256 {
		return fmt.Errorf("restore: dump integrity check failed (sha256 mismatch): %w", ErrInvalidBackup)
	}

	// Schema version guard. Backups created before this field was added carry an
	// empty SchemaVersion - skip the check rather than breaking existing backups.
	if archive.manifest.SchemaVersion == "" {
		slog.WarnContext(ctx, "restore: backup predates schema-version stamping, skipping schema check")
	} else if archive.manifest.SchemaVersion != currentSchemaVersion {
		return fmt.Errorf("restore: backup schema version %q does not match current schema %q; restore aborted to avoid corruption",
			archive.manifest.SchemaVersion, currentSchemaVersion)
	}

	f, err := os.Open(archive.dumpPath)
	if err != nil {
		return fmt.Errorf("restore: open dump temp file: %w", err)
	}
	defer f.Close()

	return s.dumper.RestoreTables(ctx, f)
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string) error {
	ctx, span := tracing.StartSpan(ctx, "app.backup.DeleteBackup")
	defer span.End()

	return s.store.Delete(ctx, backupID)
}

// StartScheduler runs an initial CleanupOldBackups then enters the periodic
// loop. The optional `initDone` channel is closed once the initial cleanup
// has returned - tests pass it to deterministically wait for the goroutine
// to make progress instead of sleeping.
func (s *Service) StartScheduler(ctx context.Context, intervalHours, retentionDays int, initDone ...chan<- struct{}) {
	if intervalHours <= 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.ErrorContext(ctx, "panic in backup scheduler", slog.Any("recover", r))
			}
		}()
		if err := s.CleanupOldBackups(ctx, retentionDays); err != nil {
			slog.WarnContext(ctx, "backup retention cleanup", slog.Any("err", err))
		}
		for _, ch := range initDone {
			if ch != nil {
				close(ch)
			}
		}
		ticker := time.NewTicker(time.Duration(intervalHours) * time.Hour)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				s.runScheduledBackupCycle(ctx, retentionDays)
			}
		}
	}()
}

// runScheduledBackupCycle performs one scheduled backup pass: create a backup
// (which announces itself, backup.failed included), then run retention cleanup.
// Extracted from the ticker loop so the failure-notification path is unit-testable
// without waiting on the hour-scale ticker.
func (s *Service) runScheduledBackupCycle(ctx context.Context, retentionDays int) {
	if _, err := s.CreateBackup(ctx); err != nil {
		// CreateBackup already published backup.failed for alerting.
		slog.WarnContext(ctx, "scheduled backup failed", slog.Any("err", err))
	} else {
		slog.Info("scheduled backup created")
	}
	if err := s.CleanupOldBackups(ctx, retentionDays); err != nil {
		slog.WarnContext(ctx, "backup retention cleanup", slog.Any("err", err))
	}
}

func (s *Service) CleanupOldBackups(ctx context.Context, retentionDays int) error {
	ctx, span := tracing.StartSpan(ctx, "app.backup.CleanupOldBackups")
	defer span.End()

	if retentionDays <= 0 {
		return nil
	}
	list, err := s.store.List(ctx)
	if err != nil {
		return fmt.Errorf("Service.CleanupOldBackups: %w", err)
	}
	cutoff := time.Now().UTC().AddDate(0, 0, -retentionDays)
	for _, m := range list {
		if m.CreatedAt.Before(cutoff) {
			if delErr := s.store.Delete(ctx, m.ID); delErr != nil {
				return fmt.Errorf("delete backup %s: %w", m.ID, delErr)
			}
		}
	}
	return nil
}

type backupArchive struct {
	manifestJSON []byte
	manifest     *backup.BackupManifest
	dumpPath     string
	dumpSHA256   string
}

func extractTarGz(r io.Reader) (*backupArchive, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, fmt.Errorf("not a valid gzip archive: %w", ErrInvalidBackup)
	}
	defer func() {
		if closeErr := gz.Close(); closeErr != nil {
			slog.Debug("close gzip reader", slog.Any("err", closeErr))
		}
	}()

	tr := tar.NewReader(gz)
	a := &backupArchive{}

	for {
		hdr, tarErr := tr.Next()
		if tarErr == io.EOF {
			break
		}
		if tarErr != nil {
			return nil, fmt.Errorf("read tar: %w", ErrInvalidBackup)
		}
		switch hdr.Name {
		case "manifest.json":
			const maxManifest = 16 << 20 // 16 MiB - a real manifest is a few KB
			content, readErr := io.ReadAll(io.LimitReader(tr, maxManifest+1))
			if readErr != nil {
				return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, ErrInvalidBackup)
			}
			if len(content) > maxManifest {
				return nil, fmt.Errorf("manifest.json too large (> 16 MiB), refusing: %w", ErrInvalidBackup)
			}
			a.manifestJSON = content
		case "dump.sql":
			tmp, tmpErr := os.CreateTemp("", "evey-restore-*.sql")
			if tmpErr != nil {
				// OS-level temp file creation failure - internal error, not bad input.
				return nil, fmt.Errorf("create temp file for dump: %w", tmpErr)
			}
			h := sha256.New()
			// Bound the decompressed dump to guard against a gzip bomb (G110): the
			// dump is row data only (metadata, never the media files themselves),
			// so even a large catalogue stays far below this. A tiny upload that
			// expands past it is corrupt or hostile - refuse rather than fill disk.
			written, cpErr := io.Copy(io.MultiWriter(tmp, h), io.LimitReader(tr, maxRestoreDumpBytes+1))
			if cpErr != nil {
				tmp.Close()
				os.Remove(tmp.Name()) //nolint:errcheck
				return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, ErrInvalidBackup)
			}
			if written > maxRestoreDumpBytes {
				tmp.Close()
				os.Remove(tmp.Name()) //nolint:errcheck
				return nil, fmt.Errorf("dump.sql too large (> 8 GiB), refusing: %w", ErrInvalidBackup)
			}
			// The integrity SHA is computed from the stream, but the restore reads
			// it back from this file. A dropped Close error could leave the file
			// truncated (e.g. a deferred flush hitting ENOSPC) while the stream SHA
			// still matches the manifest - the corruption would pass the integrity
			// check. Fail closed so a non-persisted dump never reaches the database.
			if closeErr := tmp.Close(); closeErr != nil {
				os.Remove(tmp.Name()) //nolint:errcheck
				return nil, fmt.Errorf("flush temp dump file for restore: %w", closeErr)
			}
			a.dumpPath = tmp.Name()
			a.dumpSHA256 = hex.EncodeToString(h.Sum(nil))
		}
	}

	if a.manifestJSON == nil {
		if a.dumpPath != "" {
			os.Remove(a.dumpPath) //nolint:errcheck
		}
		return nil, fmt.Errorf("archive missing manifest.json: %w", ErrInvalidBackup)
	}
	if a.dumpPath == "" {
		return nil, fmt.Errorf("archive missing dump.sql: %w", ErrInvalidBackup)
	}

	var m backup.BackupManifest
	if err := json.Unmarshal(a.manifestJSON, &m); err != nil {
		os.Remove(a.dumpPath) //nolint:errcheck
		return nil, fmt.Errorf("parse manifest: %w", ErrInvalidBackup)
	}
	a.manifest = &m
	return a, nil
}
