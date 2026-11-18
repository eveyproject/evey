package metadata

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	domainlib "github.com/eveyproject/evey/internal/domain/library"
	domain "github.com/eveyproject/evey/internal/domain/metadata"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// seriesSearchCounter counts the searches for one title, slow enough that
// concurrent callers overlap.
type seriesSearchCounter struct {
	*fakeProvider
	title string
	n     atomic.Int32
}

func (c *seriesSearchCounter) Search(ctx context.Context, q string, year *int, lang string) ([]domain.MetadataResult, error) {
	if q == c.title {
		c.n.Add(1)
		time.Sleep(50 * time.Millisecond)
	}
	return c.fakeProvider.Search(ctx, q, year, lang)
}

// The seasons and episodes of a series that just arrived each enrich the
// series first. Run in parallel by the enrich workers, they used to enrich it
// once each: as many provider searches, and concurrent writes of the same row
// (prod: a duplicate key on media_trailers_pkey). One enrichment at a time.
func TestAutoEnrich_TheSeriesOfConcurrentSeasonsIsEnrichedOnce(t *testing.T) {
	primary := &seriesSearchCounter{fakeProvider: &fakeProvider{name: "tmdb"}, title: "Show"}
	_, artRepo, personRepo, mpRepo, miRepo, searchIdx := newTestEnricher(&fakeProvider{name: "tmdb"}, nil, nil)
	e := NewEnricher(artRepo, personRepo, mpRepo, miRepo, miRepo, searchIdx, primary, nil, nil, nil)

	series := &domainlib.MediaItem{ID: "series-1", Title: "Show", Type: domainlib.Series}
	miRepo.Create(context.Background(), series)
	var seasons []*domainlib.MediaItem
	for i := 1; i <= 6; i++ {
		n := i
		s := &domainlib.MediaItem{ID: shared.MediaItemID(fmt.Sprintf("season-%d", i)), ParentID: "series-1", Type: domainlib.Season, Title: "S", IndexNumber: &n}
		miRepo.Create(context.Background(), s)
		seasons = append(seasons, s)
	}

	var wg sync.WaitGroup
	for _, s := range seasons {
		wg.Go(func() { e.AutoEnrichItem(context.Background(), s, slog.Default()) })
	}
	wg.Wait()
	assert.Equal(t, int32(1), primary.n.Load(), "the series was searched once per season instead of once")
}
