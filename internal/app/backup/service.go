package backup

import (
	"bytes"
	"context"
	"fmt"
	"io"
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

	var reader io.Reader = data
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
