package subtitle

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/subtitle"
	"github.com/eveyproject/evey/internal/domain/subtitle/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeMediaGetter struct {
	items map[string]*subtitle.MediaItemInfo
}

func (f *fakeMediaGetter) GetMediaItem(_ context.Context, id string) (*subtitle.MediaItemInfo, error) {
	if item, ok := f.items[id]; ok {
		return item, nil
	}
	return nil, fmt.Errorf("not found")
}

func newTestService() (*Service, *fakes.FakeRepository, *fakes.FakeProvider, *fakeMediaGetter) {
	repo := fakes.NewFakeRepository()
	provider := fakes.NewFakeProvider()
	media := &fakeMediaGetter{items: map[string]*subtitle.MediaItemInfo{
		"m1": {ID: "m1", FilePath: "/media/movie.mkv", Title: "Test Movie"},
	}}
	svc := NewService(repo, provider, media)
	return svc, repo, provider, media
}

func TestSearch(t *testing.T) {
	svc, _, provider, _ := newTestService()
	provider.Results = []subtitle.SearchResult{
		{FileID: 100, FileName: "test.srt", Language: "en", Format: "srt", Rating: 8.5},
	}

	results, err := svc.Search(context.Background(), "m1", "en")
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, 100, results[0].FileID)
	assert.Equal(t, "en", results[0].Language)
}

func TestSearch_MediaNotFound(t *testing.T) {
	svc, _, _, _ := newTestService()
	_, err := svc.Search(context.Background(), "nonexistent", "en")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestSearch_ProviderError(t *testing.T) {
	svc, _, provider, _ := newTestService()
	provider.Err = fmt.Errorf("rate limited")

	_, err := svc.Search(context.Background(), "m1", "en")
	assert.ErrorContains(t, err, "rate limited")
}

func TestDownload(t *testing.T) {
	svc, repo, provider, _ := newTestService()
	provider.DownloadPath = "/media/.subtitles/100.srt"

	sub, err := svc.Download(context.Background(), "m1", 100, "en", "srt")
	require.NoError(t, err)
	assert.Equal(t, shared.MediaItemID("m1"), sub.MediaItemID)
	assert.Equal(t, "en", sub.Language)
	assert.Equal(t, "srt", sub.Format)
	assert.Equal(t, "/media/.subtitles/100.srt", sub.FilePath)
	assert.Equal(t, "opensubtitles", sub.Source)

	saved, err := repo.GetByMediaItem(context.Background(), "m1")
	require.NoError(t, err)
	assert.Len(t, saved, 1)
	assert.Equal(t, sub.ID, saved[0].ID)
}

func TestDownload_MediaNotFound(t *testing.T) {
	svc, _, _, _ := newTestService()
	_, err := svc.Download(context.Background(), "nonexistent", 100, "en", "srt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "get media item")
}

func TestDownload_ProviderError(t *testing.T) {
	svc, _, provider, _ := newTestService()
	provider.Err = fmt.Errorf("download failed")

	_, err := svc.Download(context.Background(), "m1", 100, "en", "srt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "download subtitle")
}

func TestDownload_DefaultFormat(t *testing.T) {
	svc, _, provider, _ := newTestService()
	provider.DownloadPath = "/media/.subtitles/200.srt"

	sub, err := svc.Download(context.Background(), "m1", 200, "fr", "")
	require.NoError(t, err)
	assert.Equal(t, "srt", sub.Format)
}

func TestList(t *testing.T) {
	svc, repo, _, _ := newTestService()
	_ = repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub-1", MediaItemID: "m1", Language: "en", Format: "srt",
	})
	_ = repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub-2", MediaItemID: "m1", Language: "fr", Format: "srt",
	})
	_ = repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub-3", MediaItemID: "m2", Language: "en", Format: "srt",
	})

	subs, err := svc.List(context.Background(), "m1")
	require.NoError(t, err)
	assert.Len(t, subs, 2)
}

func TestDelete(t *testing.T) {
	svc, repo, _, _ := newTestService()
	_ = repo.Save(context.Background(), &subtitle.SubtitleDownload{
		ID: "sub-1", MediaItemID: "m1", Language: "en",
	})

	err := svc.Delete(context.Background(), "sub-1")
	require.NoError(t, err)

	subs, _ := repo.GetByMediaItem(context.Background(), "m1")
	assert.Empty(t, subs)
}

func TestDelete_NotFound(t *testing.T) {
	svc, _, _, _ := newTestService()
	err := svc.Delete(context.Background(), "nonexistent")
	assert.ErrorIs(t, err, shared.ErrNotFound)
}

func TestDownload_RepoSaveError(t *testing.T) {
	svc, repo, provider, _ := newTestService()
	provider.DownloadPath = "/media/.subtitles/100.srt"
	repo.Err = fmt.Errorf("db write failed")

	_, err := svc.Download(context.Background(), "m1", 100, "en", "srt")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "save subtitle")
}

func TestComputeOpenSubtitlesHash_EmptyPath(t *testing.T) {
	h := computeOpenSubtitlesHash("")
	assert.Empty(t, h)
}

func TestComputeOpenSubtitlesHash_NonExistent(t *testing.T) {
	h := computeOpenSubtitlesHash("/nonexistent/file.mkv")
	assert.Empty(t, h)
}

func TestComputeOpenSubtitlesHash_FileTooSmall(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/small.mkv"
	data := make([]byte, 1000)
	require.NoError(t, os.WriteFile(path, data, 0644))

	h := computeOpenSubtitlesHash(path)
	assert.Empty(t, h, "file smaller than 128KB should return empty hash")
}

func TestComputeOpenSubtitlesHash_ValidFile(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/video.mkv"
	size := 65536*2 + 1024
	data := make([]byte, size)
	for i := range data {
		data[i] = byte(i % 256)
	}
	require.NoError(t, os.WriteFile(path, data, 0644))

	h := computeOpenSubtitlesHash(path)
	assert.NotEmpty(t, h, "valid file should produce a hash")
	assert.Len(t, h, 16, "hash should be 16 hex characters")
}

func TestComputeOpenSubtitlesHash_Deterministic(t *testing.T) {
	tmp := t.TempDir()
	path := tmp + "/video.mkv"
	size := 65536*2 + 2048
	data := make([]byte, size)
	for i := range data {
		data[i] = byte((i * 7) % 256)
	}
	require.NoError(t, os.WriteFile(path, data, 0644))

	h1 := computeOpenSubtitlesHash(path)
	h2 := computeOpenSubtitlesHash(path)
	assert.Equal(t, h1, h2, "hash should be deterministic")
}
