package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/backup"
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
}

func NewService(store backup.BackupStore, dumper backup.DatabaseDumper, version string) *Service {
	return &Service{store: store, dumper: dumper, version: version}
}

func (s *Service) CreateBackup(ctx context.Context) (*backup.BackupManifest, error) {
	data, err := s.dumper.DumpTables(ctx, BackupTables)
	if err != nil {
		return nil, err
	}

	dumpBytes, err := io.ReadAll(data)
	if err != nil {
		return nil, err
	}

	hash := sha256.Sum256(dumpBytes)
	m := &backup.BackupManifest{
		ID:         id.New("bk-"),
		CreatedAt:  time.Now().UTC(),
		Version:    s.version,
		Tables:     BackupTables,
		DumpSHA256: hex.EncodeToString(hash[:]),
	}

	if err := s.store.Create(ctx, m, bytes.NewReader(dumpBytes)); err != nil {
		return nil, err
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
	raw, err := io.ReadAll(data)
	if err != nil {
		return err
	}

	archive, extractErr := extractTarGz(raw)
	if extractErr != nil {
		return fmt.Errorf("restore: %w", extractErr)
	}

	dump := archive.dump
	if archive.manifest.DumpSHA256 != "" {
		hash := sha256.Sum256(dump)
		got := hex.EncodeToString(hash[:])
		if got != archive.manifest.DumpSHA256 {
			return fmt.Errorf("restore: dump integrity check failed (sha256 mismatch)")
		}
	}

	return s.dumper.RestoreTables(ctx, bytes.NewReader(dump))
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string) error {
	return s.store.Delete(ctx, backupID)
}

func (s *Service) StartScheduler(ctx context.Context, intervalHours, retentionDays int) {
	if intervalHours <= 0 {
		return
	}
	go func() {
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
		return err
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
	dump         []byte
}

func extractTarGz(raw []byte) (*backupArchive, error) {
	gz, err := gzip.NewReader(bytes.NewReader(raw))
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
		content, readErr := io.ReadAll(tr)
		if readErr != nil {
			return nil, fmt.Errorf("read tar entry %s: %w", hdr.Name, readErr)
		}
		switch hdr.Name {
		case "manifest.json":
			a.manifestJSON = content
		case "dump.sql":
			a.dump = content
		}
	}

	if a.manifestJSON == nil {
		return nil, fmt.Errorf("archive missing manifest.json")
	}
	if a.dump == nil {
		return nil, fmt.Errorf("archive missing dump.sql")
	}

	var m backup.BackupManifest
	if err := json.Unmarshal(a.manifestJSON, &m); err != nil {
		return nil, fmt.Errorf("parse manifest: %w", err)
	}
	a.manifest = &m
	return a, nil
}
