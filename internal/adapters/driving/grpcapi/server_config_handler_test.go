package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/gravity-zero/evey/gen/evey/v1"
	appconfig "github.com/gravity-zero/evey/internal/app/config"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func newTestServerConfigHandler() (*serverConfigHandler, *sharedfakes.FakeServerConfigRepository) {
	repo := sharedfakes.NewFakeServerConfigRepository()
	return NewServerConfigHandler(appconfig.NewService(repo)), repo
}

func TestServerConfigHandler_GetEntry_EmptyKey(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.GetServerConfigEntry(context.Background(), &eveyv1.GetServerConfigEntryRequest{Key: ""})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestServerConfigHandler_GetEntry_NotFound(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.GetServerConfigEntry(context.Background(), &eveyv1.GetServerConfigEntryRequest{Key: "missing"})

	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestServerConfigHandler_SetAndGet(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.SetServerConfigEntry(context.Background(), &eveyv1.SetServerConfigEntryRequest{Key: "theme", Value: "dark"})
	require.NoError(t, err)

	resp, err := h.GetServerConfigEntry(context.Background(), &eveyv1.GetServerConfigEntryRequest{Key: "theme"})
	require.NoError(t, err)
	assert.Equal(t, "theme", resp.Key)
	assert.Equal(t, "dark", resp.Value)
}

func TestServerConfigHandler_SetEntry_EmptyKey(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.SetServerConfigEntry(context.Background(), &eveyv1.SetServerConfigEntryRequest{Key: "", Value: "v"})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestServerConfigHandler_ListEntries_Empty(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	resp, err := h.ListServerConfigEntries(context.Background(), &eveyv1.ListServerConfigEntriesRequest{})

	require.NoError(t, err)
	assert.Empty(t, resp.Entries)
}

func TestServerConfigHandler_ListEntries_WithData(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.SetServerConfigEntry(context.Background(), &eveyv1.SetServerConfigEntryRequest{Key: "k1", Value: "v1"})
	require.NoError(t, err)
	_, err = h.SetServerConfigEntry(context.Background(), &eveyv1.SetServerConfigEntryRequest{Key: "k2", Value: "v2"})
	require.NoError(t, err)

	resp, err := h.ListServerConfigEntries(context.Background(), &eveyv1.ListServerConfigEntriesRequest{})
	require.NoError(t, err)
	assert.Len(t, resp.Entries, 2)
}

func TestServerConfigHandler_DeleteEntry_EmptyKey(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.DeleteServerConfigEntry(context.Background(), &eveyv1.DeleteServerConfigEntryRequest{Key: ""})

	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

func TestServerConfigHandler_DeleteEntry_Success(t *testing.T) {
	h, _ := newTestServerConfigHandler()

	_, err := h.SetServerConfigEntry(context.Background(), &eveyv1.SetServerConfigEntryRequest{Key: "k1", Value: "v1"})
	require.NoError(t, err)

	_, err = h.DeleteServerConfigEntry(context.Background(), &eveyv1.DeleteServerConfigEntryRequest{Key: "k1"})
	require.NoError(t, err)

	_, err = h.GetServerConfigEntry(context.Background(), &eveyv1.GetServerConfigEntryRequest{Key: "k1"})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestServerConfigHandler_SetEntry_RepoError(t *testing.T) {
	h, repo := newTestServerConfigHandler()
	repo.Err = errors.New("db error")

	_, err := h.SetServerConfigEntry(context.Background(), &eveyv1.SetServerConfigEntryRequest{Key: "k", Value: "v"})

	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
