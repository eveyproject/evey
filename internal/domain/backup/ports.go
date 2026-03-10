package backup

import (
	"context"
	"io"
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
