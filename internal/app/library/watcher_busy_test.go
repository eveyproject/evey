package library

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type busyScanner struct{ err error }

func (s busyScanner) ScanLibrary(context.Context, string) (int, int, error) { return 0, 0, s.err }

// A scan already running is the expected case the debounce re-arms for, not a
// failure: it must not land in the ERROR stream (prod: one ERROR every 2 s
// while a long scan ran, each followed by the re-armed scan completing).
func TestWatcherService_TriggerScan_BusyIsNotAnError(t *testing.T) {
	var buf bytes.Buffer
	ws := &WatcherService{
		scanner: busyScanner{err: fmt.Errorf("%w for library lib-1", ErrScanInProgress)},
		log:     slog.New(slog.NewTextHandler(&buf, nil)),
	}
	err := ws.triggerScan(context.Background(), "lib-1")
	require.ErrorIs(t, err, ErrScanInProgress, "the caller still needs it to re-arm")
	assert.NotContains(t, buf.String(), "level=ERROR")

	buf.Reset()
	ws.scanner = busyScanner{err: errors.New("disk gone")}
	_ = ws.triggerScan(context.Background(), "lib-1")
	assert.Contains(t, buf.String(), "level=ERROR", "a real failure stays an ERROR")
}
