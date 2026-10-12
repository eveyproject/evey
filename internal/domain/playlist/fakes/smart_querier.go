package fakes

import (
	"context"
	"sort"
	"strings"

	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeSmartPlaylistQuerier struct {
	Candidates []playlist.Candidate
	Err        error
}

func NewFakeSmartPlaylistQuerier() *FakeSmartPlaylistQuerier {
	return &FakeSmartPlaylistQuerier{}
}

func (q *FakeSmartPlaylistQuerier) Query(_ context.Context, spec playlist.Spec, sortBy, sortOrder string, limit int) ([]shared.MediaItemID, error) {
	if q.Err != nil {
		return nil, q.Err
	}
	matched := make([]playlist.Candidate, 0, len(q.Candidates))
	for _, c := range q.Candidates {
		if spec == nil || spec.IsSatisfiedBy(c) {
			matched = append(matched, c)
		}
	}
	sort.Slice(matched, func(i, j int) bool {
		desc := strings.EqualFold(sortOrder, "desc")
		switch sortBy {
		case "title":
			if desc {
				return matched[i].SortTitle > matched[j].SortTitle
			}
			return matched[i].SortTitle < matched[j].SortTitle
		case "year":
			yi, yj := valueOrZero(matched[i].Year), valueOrZero(matched[j].Year)
			if desc {
				return yi > yj
			}
			return yi < yj
		case "rating":
			ri, rj := valueOrZeroF(matched[i].CommunityRating), valueOrZeroF(matched[j].CommunityRating)
			if desc {
				return ri > rj
			}
			return ri < rj
		default:
			if desc {
				return matched[i].AddedAt.After(matched[j].AddedAt)
			}
			return matched[i].AddedAt.Before(matched[j].AddedAt)
		}
	})
	if limit > 0 && len(matched) > limit {
		matched = matched[:limit]
	}
	ids := make([]shared.MediaItemID, 0, len(matched))
	for _, c := range matched {
		ids = append(ids, c.ID)
	}
	return ids, nil
}

func valueOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

func valueOrZeroF(p *float64) float64 {
	if p == nil {
		return 0
	}
	return *p
}
