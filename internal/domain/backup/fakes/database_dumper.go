package fakes

import (
	"bytes"
	"context"
	"io"
	"sync"
)

type FakeDatabaseDumper struct {
	mu               sync.Mutex
	DumpData         []byte
	Restored         []byte
	Err              error
	SchemaVersion    string
	SchemaVersionErr error
	// LastIncludeTelemetry records the last value DumpAll was called with, so a
	// test can assert the service forwarded the setting.
	LastIncludeTelemetry bool
}

func NewFakeDatabaseDumper() *FakeDatabaseDumper {
	return &FakeDatabaseDumper{DumpData: []byte("-- fake dump\n")}
}

func (d *FakeDatabaseDumper) DumpAll(_ context.Context, includeTelemetry bool) (io.ReadCloser, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	d.LastIncludeTelemetry = includeTelemetry
	if d.Err != nil {
		return nil, d.Err
	}
	return io.NopCloser(bytes.NewReader(d.DumpData)), nil
}

func (d *FakeDatabaseDumper) RestoreTables(_ context.Context, data io.Reader) error {
	if d.Err != nil {
		return d.Err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.Err != nil {
		return d.Err
	}
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	d.Restored = b
	return nil
}

func (d *FakeDatabaseDumper) CurrentSchemaVersion(_ context.Context) (string, error) {
	if d.Err != nil {
		return "", d.Err
	}
	if d.SchemaVersionErr != nil {
		return "", d.SchemaVersionErr
	}
	return d.SchemaVersion, nil
}
