package library_test

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	applib "github.com/eveyproject/evey/internal/app/library"
	"github.com/eveyproject/evey/internal/domain/library"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	mkvfakes "github.com/eveyproject/evey/internal/domain/mkv/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// captureDefaultSlog redirects slog.Default while fn runs. NOT safe for t.Parallel.
func captureDefaultSlog(t *testing.T, fn func()) string {
	t.Helper()
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug})))
	defer slog.SetDefault(prev)
	fn()
	return buf.String()
}

func newCmdSvcForMutations() (*applib.CommandService, *libfakes.FakeLibraryCommandRepo, *libfakes.FakeLibraryQueryRepo, *libfakes.FakeMediaItemCommandRepo, *sharedfakes.FakeEventBus) {
	libCmd := libfakes.NewFakeLibraryCommandRepo()
	libQuery := libfakes.NewFakeLibraryQueryRepo()
	mediaRepo := libfakes.NewFakeMediaItemCommandRepo()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	prober := libfakes.NewFakeStreamProber()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()
	svc := applib.NewCommandService(libCmd, libQuery, mediaRepo, streamRepo, personRepo, mediaPersonRepo,
		scanner, prober, search, bus,
		libfakes.NewFakeChapterRepository(), mkvfakes.NewFakeContainerReader(),
		libfakes.NewFakeTrickplayRepository(), libfakes.NewFakeTrickplayGenerator(), "/cache", nil,
	)
	return svc, libCmd, libQuery, mediaRepo, bus
}

// Kills command_service.go CreateLibrary publish error NEGATION -
// when Publish fails, a Warn log must be emitted. Without it, flipping
// `err != nil` to `err == nil` stays undetected.
func TestCreateLibrary_PublishError_LogsWarn(t *testing.T) {
	svc, _, _, _, bus := newCmdSvcForMutations()
	bus.PublishErr = errors.New("bus gone")

	logs := captureDefaultSlog(t, func() {
		err := svc.CreateLibrary(context.Background(), &library.Library{ID: "l1", Name: "N", Type: "movie"})
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "publish event")
	assert.Contains(t, logs, "bus gone")
	assert.Contains(t, logs, "library.created")
}

// Complementary: without publish error, no warn must be logged (kills negation on the other side).
func TestCreateLibrary_NoPublishError_NoWarn(t *testing.T) {
	svc, _, _, _, _ := newCmdSvcForMutations()

	logs := captureDefaultSlog(t, func() {
		err := svc.CreateLibrary(context.Background(), &library.Library{ID: "l1", Name: "N", Type: "movie"})
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "publish event")
}

// Kills DeleteLibrary publish error NEGATION.
func TestDeleteLibrary_PublishError_LogsWarn(t *testing.T) {
	svc, libCmd, _, _, bus := newCmdSvcForMutations()
	libCmd.Create(context.Background(), &library.Library{ID: "l-del"})
	bus.PublishErr = errors.New("bus down")

	logs := captureDefaultSlog(t, func() {
		err := svc.DeleteLibrary(context.Background(), "l-del")
		require.NoError(t, err)
	})
	assert.Contains(t, logs, "publish event")
	assert.Contains(t, logs, "bus down")
	assert.Contains(t, logs, "library.deleted")
}

func TestDeleteLibrary_NoPublishError_NoWarn(t *testing.T) {
	svc, libCmd, _, _, _ := newCmdSvcForMutations()
	libCmd.Create(context.Background(), &library.Library{ID: "l-del"})

	logs := captureDefaultSlog(t, func() {
		err := svc.DeleteLibrary(context.Background(), "l-del")
		require.NoError(t, err)
	})
	assert.NotContains(t, logs, "publish event")
}

// Kills CreateLibrary sets UpdatedAt/CreatedAt - ensure the time is actually written.
// NEGATION on `if err := Create; err != nil` would not be killed here, but verifying
// the event OccurredAt matches lib.CreatedAt catches the `now := time.Now()` reordering.
func TestCreateLibrary_TimestampsMatchEvent(t *testing.T) {
	svc, _, _, _, bus := newCmdSvcForMutations()
	lib := &library.Library{ID: "l1", Name: "N", Type: "movie"}
	before := time.Now().Add(-time.Second)

	err := svc.CreateLibrary(context.Background(), lib)
	require.NoError(t, err)

	assert.True(t, lib.CreatedAt.After(before))
	assert.True(t, lib.UpdatedAt.After(before))
	assert.Equal(t, lib.CreatedAt, lib.UpdatedAt)

	events := bus.PublishedEvents()
	require.Len(t, events, 1)
	evt := events[0].(library.LibraryCreated)
	assert.Equal(t, lib.CreatedAt, evt.OccurredAt)
}
