package library_test

import (
	"context"
	"testing"

	applib "github.com/gravity-zero/evey/internal/app/library"
	"github.com/gravity-zero/evey/internal/domain/library"
	libfakes "github.com/gravity-zero/evey/internal/domain/library/fakes"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
)

func TestCreateLibrary(t *testing.T) {
	libRepo := libfakes.NewFakeLibraryRepository()
	mediaRepo := libfakes.NewFakeMediaItemRepository()
	streamRepo := libfakes.NewFakeMediaStreamRepository()
	personRepo := libfakes.NewFakePersonRepository()
	mediaPersonRepo := libfakes.NewFakeMediaPersonRepository()
	scanner := libfakes.NewFakeFileScanner()
	search := libfakes.NewFakeSearchIndex()
	bus := sharedfakes.NewFakeEventBus()

	svc := applib.NewService(libRepo, mediaRepo, streamRepo, personRepo, mediaPersonRepo, scanner, search, bus)

	lib := &library.Library{
		ID:   "lib-1",
		Name: "Movies",
		Type: "movie",
	}
	err := svc.CreateLibrary(context.Background(), lib)
	assert.NoError(t, err)

	got, err := svc.GetLibrary(context.Background(), "lib-1")
	assert.NoError(t, err)
	assert.Equal(t, "Movies", got.Name)
	assert.Len(t, bus.PublishedEvents(), 1)
}
