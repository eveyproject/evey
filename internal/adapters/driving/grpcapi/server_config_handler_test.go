package grpcapi

import (
	"context"
	"errors"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appconfig "github.com/evey-project/evey/internal/app/config"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
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

func TestServerConfigHandler_SetTranslator_SetsField(t *testing.T) {
	h, _ := newTestServerConfigHandler()
	if h.translator != nil {
		t.Fatal("expected nil translator before SetTranslator")
	}
	tr := eveyi18n.NewEmpty()
	h.SetTranslator(tr)
	if h.translator != tr {
		t.Error("SetTranslator did not assign translator")
	}
}

// A credential must never leave the server. The store is a generic key/value
// table so it had no notion of a secret: the panel received the TMDB key and
// the SMTP password in clear and masked them with bullets in the DOM, with the
// full value sitting in the JSON right behind it.
func TestServerConfigHandler_SecretsAreMaskedOnRead(t *testing.T) {
	h, repo := newTestServerConfigHandler()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, "api.tmdb_key", "real-tmdb-key-abcdef"))
	require.NoError(t, repo.Set(ctx, "smtp.password", "hunter2"))
	require.NoError(t, repo.Set(ctx, "smtp.host", "smtp.example.com"))

	got, err := h.GetServerConfigEntry(ctx, &eveyv1.GetServerConfigEntryRequest{Key: "api.tmdb_key"})
	require.NoError(t, err)
	assert.Equal(t, appconfig.SecretMask, got.Value)

	resp, err := h.ListServerConfigEntries(ctx, &eveyv1.ListServerConfigEntriesRequest{})
	require.NoError(t, err)
	values := map[string]string{}
	for _, e := range resp.Entries {
		values[e.Key] = e.Value
	}
	assert.Equal(t, appconfig.SecretMask, values["api.tmdb_key"])
	assert.Equal(t, appconfig.SecretMask, values["smtp.password"])
	// The host is not a credential and the panel still has to show it.
	assert.Equal(t, "smtp.example.com", values["smtp.host"])

	// Nothing anywhere in the response carries the real values.
	for _, e := range resp.Entries {
		assert.NotContains(t, e.Value, "real-tmdb-key")
		assert.NotContains(t, e.Value, "hunter2")
	}
}

// An unset secret must not be reported as configured - otherwise the operator
// debugs an integration that was never set up.
func TestServerConfigHandler_UnsetSecretStaysEmpty(t *testing.T) {
	h, repo := newTestServerConfigHandler()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, "api.omdb_key", ""))

	got, err := h.GetServerConfigEntry(ctx, &eveyv1.GetServerConfigEntryRequest{Key: "api.omdb_key"})
	require.NoError(t, err)
	assert.Empty(t, got.Value)
}

// Write-only semantics: the SMTP card submits all six fields on every save, so
// echoing back the mask the server just served must leave the stored password
// alone instead of overwriting it with bullets.
func TestServerConfigHandler_SetSecret_MaskKeepsStoredValue(t *testing.T) {
	h, repo := newTestServerConfigHandler()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, "smtp.password", "hunter2"))

	_, err := h.SetServerConfigEntry(ctx, &eveyv1.SetServerConfigEntryRequest{Key: "smtp.password", Value: appconfig.SecretMask})
	require.NoError(t, err)

	stored, err := repo.Get(ctx, "smtp.password")
	require.NoError(t, err)
	assert.Equal(t, "hunter2", stored.Value)
}

// A real new value still gets written, and clearing the field still removes the
// credential - the mask is the only value with a special meaning.
func TestServerConfigHandler_SetSecret_RealValueAndClearStillWork(t *testing.T) {
	h, repo := newTestServerConfigHandler()
	ctx := context.Background()
	require.NoError(t, repo.Set(ctx, "smtp.password", "hunter2"))

	_, err := h.SetServerConfigEntry(ctx, &eveyv1.SetServerConfigEntryRequest{Key: "smtp.password", Value: "correct horse"})
	require.NoError(t, err)
	stored, err := repo.Get(ctx, "smtp.password")
	require.NoError(t, err)
	assert.Equal(t, "correct horse", stored.Value)

	_, err = h.SetServerConfigEntry(ctx, &eveyv1.SetServerConfigEntryRequest{Key: "smtp.password", Value: ""})
	require.NoError(t, err)
	stored, err = repo.Get(ctx, "smtp.password")
	require.NoError(t, err)
	assert.Empty(t, stored.Value)
}
