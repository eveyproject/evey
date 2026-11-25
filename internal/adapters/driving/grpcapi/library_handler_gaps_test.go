package grpcapi

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appcatalog "github.com/eveyproject/evey/internal/app/catalog"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/auth"
)

// Who may read the gaps: an admin session, or a plugin token carrying
// media.gaps; a user session or a plugin token without it may not.
func TestMayReadGaps(t *testing.T) {
	bg := context.Background()
	assert.False(t, mayReadGaps(bg))
	assert.False(t, mayReadGaps(interceptors.ContextWithClaims(bg, &user.Claims{UserID: "u", IsAdmin: false})))
	assert.True(t, mayReadGaps(interceptors.ContextWithClaims(bg, &user.Claims{UserID: "u", IsAdmin: true})))
	assert.False(t, mayReadGaps(interceptors.ContextWithPluginClaims(bg, &auth.PluginClaims{Scopes: []string{"public", "media.files"}})))
	assert.True(t, mayReadGaps(interceptors.ContextWithPluginClaims(bg, &auth.PluginClaims{Scopes: []string{"media.gaps"}})))
}

// The proto carries numbers, titles and dates; an unknown date is empty.
func TestCatalogGapsToProto(t *testing.T) {
	d, _ := time.Parse("2006-01-02", "2012-04-01")
	got := catalogGapsToProto(appcatalog.Gaps{
		Series: []appcatalog.SeriesGaps{{SeriesID: "s", Title: "GoT", Year: 2011, FromCatalog: true, Seasons: []appcatalog.MissingSeason{
			{Season: 2, Episodes: []appcatalog.MissingEpisode{{Episode: 1, Title: "The North Remembers", AirDate: &d}, {Episode: 5}}},
		}}},
		Sagas: []appcatalog.SagaGaps{{CollectionID: "c", Name: "Rocky", Owned: 2, Missing: []library.CollectionPart{{Title: "Rocky III", ReleaseDate: &d, PosterURL: "https://p"}}}},
	})
	require.Len(t, got.Series, 1)
	assert.Equal(t, "2012-04-01", got.Series[0].Seasons[0].Episodes[0].AirDate)
	assert.Empty(t, got.Series[0].Seasons[0].Episodes[1].AirDate)
	require.Len(t, got.Sagas, 1)
	assert.Equal(t, "Rocky III", got.Sagas[0].Missing[0].Title)
}
