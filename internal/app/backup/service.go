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

var BackupTables = []string{
	"users",
	"profiles",
	"libraries",
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
	"feed_configs",
	"webhooks",
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
}

type Service struct {
	store   backup.BackupStore
	dumper  backup.DatabaseDumper
	version string
	events  shared.EventBus
}

type Option func(*Service)

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

func (s *Service) CreateBackup(ctx context.Context) (*backup.BackupManifest, error) {
	ctx, span := tracing.StartSpan(ctx, "app.backup.CreateBackup")
	defer span.End()

	data, err := s.dumper.DumpTables(ctx, BackupTables)
	if err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: %w", err)
	}

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
// has returned — tests pass it to deterministically wait for the goroutine
// to make progress instead of sleeping.
func (s *Service) StartScheduler(ctx context.Context, intervalHours, retentionDays int, initDone ...chan<- struct{}) {
	if intervalHours <= 0 {
		return
	}
	go func() {
		defer func() {
			if r := recover(); r != nil {
				slog.Error("panic in backup scheduler", slog.Any("recover", r))
			}
		}()
		if err := s.CleanupOldBackups(ctx, retentionDays); err != nil {
			slog.Warn("backup retention cleanup", slog.Any("err", err))
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
				if _, err := s.CreateBackup(ctx); err != nil {
					slog.Warn("scheduled backup failed", slog.Any("err", err))
					if s.events != nil {
						evt := backup.BackupFailed{
							Error:      err.Error(),
							OccurredAt: time.Now(),
						}
						if pubErr := s.events.Publish(ctx, evt); pubErr != nil {
							slog.Warn("publish backup.failed event", slog.Any("err", pubErr))
						}
					}
				} else {
					slog.Info("scheduled backup created")
				}
				if err := s.CleanupOldBackups(ctx, retentionDays); err != nil {
					slog.Warn("backup retention cleanup", slog.Any("err", err))
				}
			}
		}
	}()
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
			content, readErr := io.ReadAll(tr)
			if readErr != nil {
				return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, ErrInvalidBackup)
			}
			a.manifestJSON = content
		case "dump.sql":
			tmp, tmpErr := os.CreateTemp("", "evey-restore-*.sql")
			if tmpErr != nil {
				// OS-level temp file creation failure — internal error, not bad input.
				return nil, fmt.Errorf("create temp file for dump: %w", tmpErr)
			}
			h := sha256.New()
			if _, cpErr := io.Copy(io.MultiWriter(tmp, h), tr); cpErr != nil {
				tmp.Close()
				os.Remove(tmp.Name()) //nolint:errcheck
				return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, ErrInvalidBackup)
			}
			tmp.Close()
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
