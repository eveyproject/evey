package backup

import (
	"context"
	"io"
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
	m := &backup.BackupManifest{
		ID:        id.New("bk-"),
		CreatedAt: time.Now().UTC(),
		Version:   s.version,
		Tables:    BackupTables,
	}
	if err := s.store.Create(ctx, m, data); err != nil {
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
	return s.dumper.RestoreTables(ctx, data)
}

func (s *Service) DeleteBackup(ctx context.Context, backupID string) error {
	return s.store.Delete(ctx, backupID)
}
