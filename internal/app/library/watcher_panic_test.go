package library

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type panickingScanner struct{}

func (panickingScanner) ScanLibrary(context.Context, string) (int, int, error) {
	panic("boom in ScanLibrary")
}

// A panic inside ScanLibrary runs on the debounce timer's own goroutine, where
// an unrecovered panic takes down the whole process. triggerScan must contain
// it and surface an error instead, staying symmetric with the scheduler, which
// already recovers a buggy scan of one library (scheduler.go runScan).
func TestWatcherService_TriggerScan_RecoversPanic(t *testing.T) {
	ws := &WatcherService{
		scanner: panickingScanner{},
		log:     slog.New(slog.NewTextHandler(os.Stderr, nil)),
	}
	var err error
	require.NotPanics(t, func() {
		err = ws.triggerScan(context.Background(), "lib-1")
	})
	assert.Error(t, err)
}
