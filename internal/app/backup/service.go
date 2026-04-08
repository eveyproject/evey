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

	"github.com/gravity-zero/evey/internal/domain/backup"
	"github.com/gravity-zero/evey/internal/domain/shared"
	"github.com/gravity-zero/evey/internal/pkg/id"
)

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

	m := &backup.BackupManifest{
		ID:         id.New("bk-"),
		CreatedAt:  time.Now().UTC(),
		Version:    s.version,
		Tables:     BackupTables,
		DumpSHA256: hex.EncodeToString(h.Sum(nil)),
	}

	if err := s.store.Create(ctx, m, tmp); err != nil {
		return nil, fmt.Errorf("Service.CreateBackup: %w", err)
	}
	return m, nil
}

func (s *Service) ListBackups(ctx context.Context) ([]*backup.BackupManifest, error) {
	return s.store.List(ctx)
}

func (s *Service) GetBackup(ctx context.Context, backupID string) (*backup.BackupManifest, io.ReadCloser, error) {
	return s.store.Get(ctx, backupID)
}

func (s *Service) RestoreBackup(ctx context.Context, data io.Reader) error {
	archive, extractErr := extractTarGz(data)
	if extractErr != nil {
		return fmt.Errorf("restore: %w", extractErr)
	}
	defer os.Remove(archive.dumpPath) //nolint:errcheck

	if archive.manifest.DumpSHA256 != "" {
		if archive.dumpSHA256 != archive.manifest.DumpSHA256 {
			return errors.New("restore: dump integrity check failed (sha256 mismatch)")
		}
	}

	f, err := os.Open(archive.dumpPath)
	if err != nil {
		return fmt.Errorf("restore: open dump temp file: %w", err)
	}
	defer f.Close()

	return s.dumper.RestoreTables(ctx, f)
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string) error {
	return s.store.Delete(ctx, backupID)
}

func (s *Service) StartScheduler(ctx context.Context, intervalHours, retentionDays int) {
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
		return nil, fmt.Errorf("not a valid gzip archive: %w", err)
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
			return nil, fmt.Errorf("read tar: %w", tarErr)
		}
		switch hdr.Name {
		case "manifest.json":
			content, readErr := io.ReadAll(tr)
			if readErr != nil {
				return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, readErr)
			}
			a.manifestJSON = content
		case "dump.sql":
			tmp, tmpErr := os.CreateTemp("", "evey-restore-*.sql")
			if tmpErr != nil {
				return nil, fmt.Errorf("create temp file for dump: %w", tmpErr)
			}
			h := sha256.New()
			if _, cpErr := io.Copy(io.MultiWriter(tmp, h), tr); cpErr != nil {
				tmp.Close()
				os.Remove(tmp.Name()) //nolint:errcheck
				return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, cpErr)
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
		return nil, errors.New("archive missing manifest.json")
	}
	if a.dumpPath == "" {
		return nil, errors.New("archive missing dump.sql")
	}

	var m backup.BackupManifest
	if err := json.Unmarshal(a.manifestJSON, &m); err != nil {
		os.Remove(a.dumpPath) //nolint:errcheck
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	a.manifest = &m
	return a, nil
}
