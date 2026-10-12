package photo_test

import (
	"context"
	"testing"
	"time"

	appphoto "github.com/eveyproject/evey/internal/app/photo"
	domainphoto "github.com/eveyproject/evey/internal/domain/photo"
	photofakes "github.com/eveyproject/evey/internal/domain/photo/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// NewService exposes both CommandService and QueryService facades.
func TestService_Facade_Wiring(t *testing.T) {
	t.Parallel()
	repo := photofakes.NewFakeRepository()
	scanner := photofakes.NewFakePhotoScanner()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	assert.NotNil(t, svc.CommandService())
	assert.NotNil(t, svc.QueryService())
}

// ListPhotosByAlbum delegates to QueryService and filters by album.
func TestService_ListPhotosByAlbum_FiltersByAlbum(t *testing.T) {
	t.Parallel()
	repo := photofakes.NewFakeRepository()
	ctx := context.Background()
	now := time.Now()
	require.NoError(t, repo.Upsert(ctx, domainphoto.Photo{ID: "p1", LibraryID: "lib-1", Album: "A", CreatedAt: now}))
	require.NoError(t, repo.Upsert(ctx, domainphoto.Photo{ID: "p2", LibraryID: "lib-1", Album: "A", CreatedAt: now}))
	require.NoError(t, repo.Upsert(ctx, domainphoto.Photo{ID: "p3", LibraryID: "lib-1", Album: "B", CreatedAt: now}))

	scanner := photofakes.NewFakePhotoScanner()
	svc := appphoto.NewService(repo, sharedfakes.NewFakeEventBus(), nil, scanner)

	photos, total, err := svc.ListPhotosByAlbum(ctx, "lib-1", "A", 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 2, total)
	assert.Len(t, photos, 2)

	_, totalB, err := svc.ListPhotosByAlbum(ctx, "lib-1", "B", 50, 0)
	require.NoError(t, err)
	assert.Equal(t, 1, totalB)
}
