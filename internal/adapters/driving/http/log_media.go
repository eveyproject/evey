package httpapi

// log_media.go - the media filter of the admin log search. Log lines carry the
// id of the item played, an episode's for a series; the operator knows a
// title. A title is looked up in the search index (Meilisearch, through the
// library's own search and its SQL fallback), and each item found brings the
// ids of everything under it, so "Stranger Things" matches every episode.
// A series is named by its own title: seasons and episodes are not looked up
// by theirs (owner decision, 2026-10-21), they come along with their series.

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"slices"
	"strings"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// logMediaLookup is what the resolver needs of the library: its search, and
// the items under an item (applibrary.QueryService).
type logMediaLookup interface {
	SearchMedia(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error)
	MediaDescendantIDs(ctx context.Context, id string) ([]string, error)
}

// logMediaMatch is one item a title resolved to, with how many items under it
// (seasons and episodes) the filter takes along.
type logMediaMatch struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Type        string `json:"type"`
	Descendants int    `json:"descendants"`
}

// logMediaResolution is what the filter matches: IDs, every media id a line
// may carry to pass, and Media, the items a title resolved to (empty when the
// input was an id, or when nothing matched: then IDs is empty too).
type logMediaResolution struct {
	Media []logMediaMatch `json:"media"`
	IDs   []string        `json:"ids"`
}

var logMediaIDRe = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// logMediaSearchHits is how many search hits are read: enough, once seasons
// and episodes are set aside, to take every item sharing the best hit's title
// (a film in two versions).
const logMediaSearchHits = 20

// resolveLogMedia turns the media filter into the ids it matches. An id, or
// any input when no lookup is wired, matches itself. A title matches the best
// search hit that is not a season or an episode, every other such hit with the
// same title, and all their descendants.
func resolveLogMedia(ctx context.Context, lk logMediaLookup, raw string) (logMediaResolution, error) {
	raw = strings.TrimSpace(raw)
	res := logMediaResolution{Media: []logMediaMatch{}, IDs: []string{}}
	if raw == "" {
		return res, nil
	}
	if lk == nil || logMediaIDRe.MatchString(raw) {
		res.IDs = []string{raw}
		return res, nil
	}
	found, _, err := lk.SearchMedia(ctx, raw, nil, nil, logMediaSearchHits)
	if err != nil {
		return res, err
	}
	hits := slices.DeleteFunc(found, func(h library.SearchHit) bool {
		return h.Type == string(library.Season) || h.Type == string(library.Episode)
	})
	if len(hits) == 0 {
		return res, nil
	}
	best := strings.ToLower(strings.TrimSpace(hits[0].Title))
	for _, h := range hits {
		if strings.ToLower(strings.TrimSpace(h.Title)) != best {
			continue
		}
		under, err := lk.MediaDescendantIDs(ctx, h.ID)
		if err != nil {
			return logMediaResolution{}, err
		}
		res.Media = append(res.Media, logMediaMatch{ID: h.ID, Title: h.Title, Type: h.Type, Descendants: len(under)})
		res.IDs = append(res.IDs, h.ID)
		res.IDs = append(res.IDs, under...)
	}
	return res, nil
}

// handleResolveLogMedia answers GET /api/v1/admin/logs/media?q=: the live
// tail filters in the browser, so it asks here which ids a title stands for;
// the explorer also shows what the title resolved to.
func (h *adminHandler) handleResolveLogMedia(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.admin.ResolveLogMedia")
	defer span.End()
	res, err := resolveLogMedia(ctx, h.mediaLookup, r.URL.Query().Get("q"))
	if err != nil {
		span.RecordError(err)
		slog.WarnContext(ctx, "admin logs: media lookup failed", slog.Any("err", err))
		http.Error(w, `{"message":"media lookup failed"}`, http.StatusBadGateway)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(res); err != nil {
		slog.WarnContext(ctx, "json encode failed", slog.Any("err", err))
	}
}
