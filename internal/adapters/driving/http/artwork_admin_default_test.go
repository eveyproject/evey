package httpapi

import (
	"context"
	"testing"

	appconfig "github.com/eveyproject/evey/internal/app/config"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// Default ON is the load-bearing UX guarantee: without it 99% of fresh
// installs see broken posters because the metadata enricher creates
// artwork rows pointing at external URLs (Amazon CDN for OMDb) that
// Firefox's Opaque Response Blocking refuses to display.
func TestResolveAutoDownloadDefault(t *testing.T) {
	t.Run("nil config service → true", func(t *testing.T) {
		if !resolveAutoDownloadDefault(context.Background(), nil) {
			t.Fatal("nil cfg must default to true")
		}
	})

	t.Run("absent key → true", func(t *testing.T) {
		cfg := appconfig.NewService(sharedfakes.NewFakeServerConfigRepository())
		if !resolveAutoDownloadDefault(context.Background(), cfg) {
			t.Fatal("missing key must default to true")
		}
	})

	t.Run("key=true → true", func(t *testing.T) {
		repo := sharedfakes.NewFakeServerConfigRepository()
		_ = repo.Set(context.Background(), configKeyArtworkAutoDownload, "true")
		cfg := appconfig.NewService(repo)
		if !resolveAutoDownloadDefault(context.Background(), cfg) {
			t.Fatal("key=\"true\" must yield true")
		}
	})

	t.Run("key=false → false (admin opt-out)", func(t *testing.T) {
		repo := sharedfakes.NewFakeServerConfigRepository()
		_ = repo.Set(context.Background(), configKeyArtworkAutoDownload, "false")
		cfg := appconfig.NewService(repo)
		if resolveAutoDownloadDefault(context.Background(), cfg) {
			t.Fatal("key=\"false\" must yield false")
		}
	})

	t.Run("garbage value → true (conservative)", func(t *testing.T) {
		repo := sharedfakes.NewFakeServerConfigRepository()
		_ = repo.Set(context.Background(), configKeyArtworkAutoDownload, "yes please")
		cfg := appconfig.NewService(repo)
		if !resolveAutoDownloadDefault(context.Background(), cfg) {
			t.Fatal("unparseable value must fall back to true rather than silently disable")
		}
	})
}
