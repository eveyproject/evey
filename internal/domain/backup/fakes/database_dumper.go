package fakes

import (
	"bytes"
	"context"
	"io"
	"sync"
)

type FakeDatabaseDumper struct {
	mu       sync.Mutex
	DumpData []byte
	Restored []byte
	Err      error
}

func NewFakeDatabaseDumper() *FakeDatabaseDumper {
	return &FakeDatabaseDumper{DumpData: []byte("-- fake dump\n")}
}

func (d *FakeDatabaseDumper) DumpTables(_ context.Context, _ []string) (io.Reader, error) {
	if d.Err != nil {
		return nil, d.Err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	return bytes.NewReader(d.DumpData), nil
}

func (d *FakeDatabaseDumper) RestoreTables(_ context.Context, data io.Reader) error {
	if d.Err != nil {
		return d.Err
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	b, err := io.ReadAll(data)
	if err != nil {
		return err
	}
	d.Restored = b
	return nil
}
