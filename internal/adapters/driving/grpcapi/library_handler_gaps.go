package grpcapi

import (
	"context"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appcatalog "github.com/eveyproject/evey/internal/app/catalog"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// mayReadGaps: a plugin token carrying media.gaps (minted on an
// administrator's session alone), or an administrator's own session.
func mayReadGaps(ctx context.Context) bool {
	if pc := interceptors.PluginClaimsFromContext(ctx); pc != nil {
		return pc.HasScope(auth.MediaGapsScope)
	}
	c := interceptors.ClaimsFromContext(ctx)
	return c != nil && c.IsAdmin
}

func (h *libraryHandler) GetCatalogGaps(ctx context.Context, _ *eveyv1.GetCatalogGapsRequest) (*eveyv1.GetCatalogGapsResponse, error) {
	ctx, span := tracing.StartSpan(ctx, "grpc.library.GetCatalogGaps")
	defer span.End()
	if h.catalog == nil {
		return nil, status.Error(codes.Unimplemented, "catalog gaps not configured")
	}
	if !mayReadGaps(ctx) {
		return nil, h.translator.TranslatedError(ctx, codes.PermissionDenied, "errors.auth.admin_access_required")
	}
	gaps, err := h.catalog.Gaps(ctx, nil)
	if err != nil {
		return nil, internalError(ctx, "catalog gaps", err)
	}
	return catalogGapsToProto(gaps), nil
}

func ymd(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.Format("2006-01-02")
}

func catalogGapsToProto(g appcatalog.Gaps) *eveyv1.GetCatalogGapsResponse {
	out := &eveyv1.GetCatalogGapsResponse{}
	for _, s := range g.Series {
		ps := &eveyv1.SeriesGaps{SeriesId: s.SeriesID, Title: s.Title, Year: int32(s.Year), FromCatalog: s.FromCatalog}
		for _, se := range s.Seasons {
			ms := &eveyv1.MissingSeason{Season: int32(se.Season), Whole: se.Whole}
			for _, e := range se.Episodes {
				ms.Episodes = append(ms.Episodes, &eveyv1.MissingEpisode{Episode: int32(e.Episode), Title: e.Title, AirDate: ymd(e.AirDate)})
			}
			ps.Seasons = append(ps.Seasons, ms)
		}
		out.Series = append(out.Series, ps)
	}
	for _, sg := range g.Sagas {
		pg := &eveyv1.SagaGaps{CollectionId: sg.CollectionID, Name: sg.Name, Owned: int32(sg.Owned)}
		for _, p := range sg.Missing {
			pg.Missing = append(pg.Missing, &eveyv1.SagaPart{Title: p.Title, ReleaseDate: ymd(p.ReleaseDate), PosterUrl: p.PosterURL})
		}
		out.Sagas = append(out.Sagas, pg)
	}
	return out
}
