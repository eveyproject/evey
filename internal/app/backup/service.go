package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"log/slog"
	"time"

	"github.com/gravity-zero/evey/internal/domain/backup"
	"github.com/gravity-zero/evey/internal/pkg/backupcrypto"
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
	"fcm_device_tokens",
}

type Service struct {
	store         backup.BackupStore
	dumper        backup.DatabaseDumper
	version       string
	encryptionKey []byte // nil = no encryption
}

func NewService(store backup.BackupStore, dumper backup.DatabaseDumper, version string) *Service {
	return &Service{store: store, dumper: dumper, version: version}
}

// WithEncryptionKey sets a 32-byte AES-256-GCM key for backup encryption.
// Generate via: openssl rand -base64 32  (set EVEY_BACKUP_ENCRYPTION_KEY)
func (s *Service) WithEncryptionKey(key []byte) {
	s.encryptionKey = key
}

func (s *Service) CreateBackup(ctx context.Context) (*backup.BackupManifest, error) {
	data, err := s.dumper.DumpTables(ctx, BackupTables)
	if err != nil {
		return nil, err
	}
	m := &backup.BackupManifest{
		ID:        id.New("bk-"),
		CreatedAt: time.Now().UTC(),
		Version:   s.version,
		Tables:    BackupTables,
		Encrypted: len(s.encryptionKey) > 0,
	}

	var reader = data
	if len(s.encryptionKey) > 0 {
		raw, readErr := io.ReadAll(data)
		if readErr != nil {
			return nil, readErr
		}
		enc, encErr := backupcrypto.Encrypt(raw, s.encryptionKey)
		if encErr != nil {
			return nil, encErr
		}
		reader = bytes.NewReader(enc)
	}

	if err := s.store.Create(ctx, m, reader); err != nil {
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
	if backupcrypto.IsEncrypted(raw) {
		if len(s.encryptionKey) == 0 {
			return fmt.Errorf("backup is encrypted but no EVEY_BACKUP_ENCRYPTION_KEY is set")
		}
		raw, err = backupcrypto.Decrypt(raw, s.encryptionKey)
		if err != nil {
			return fmt.Errorf("decrypt backup: %w", err)
		}
	}
	return s.dumper.RestoreTables(ctx, bytes.NewReader(raw))
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
