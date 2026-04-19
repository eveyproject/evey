package backup

import (
	"errors"
	"slices"
	"time"
)

type BackupManifest struct {
	ID         string    `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	Version    string    `json:"version"`
	Tables     []string  `json:"tables"`
	Size       int64     `json:"size"`
	DumpSHA256 string    `json:"dump_sha256,omitempty"`
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
