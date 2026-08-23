package meilisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	ms "github.com/meilisearch/meilisearch-go"

	"github.com/evey-project/evey/internal/domain/library"
	"github.com/evey-project/evey/internal/domain/shared"
	eveyi18n "github.com/evey-project/evey/internal/pkg/i18n"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const (
	indexName      = "media_items"
	connectTimeout = 30 * time.Second
)

type IndexClient interface {
	AddDocumentsWithContext(ctx context.Context, documentsPtr any, opts *ms.DocumentOptions) (*ms.TaskInfo, error)
	DeleteDocumentWithContext(ctx context.Context, identifier string, opts *ms.DocumentOptions) (*ms.TaskInfo, error)
	DeleteAllDocumentsWithContext(ctx context.Context, opts *ms.DocumentOptions) (*ms.TaskInfo, error)
	SearchWithContext(ctx context.Context, query string, request *ms.SearchRequest) (*ms.SearchResponse, error)
	UpdateSettingsWithContext(ctx context.Context, request *ms.Settings) (*ms.TaskInfo, error)
	GetStatsWithContext(ctx context.Context) (*ms.StatsIndex, error)
}

type ServiceClient interface {
	Index(uid string) ms.IndexManager
	CreateIndexWithContext(ctx context.Context, config *ms.IndexConfig) (*ms.TaskInfo, error)
	GetIndexWithContext(ctx context.Context, indexID string) (*ms.IndexResult, error)
}

type SearchAdapter struct {
	index IndexClient
}

func NewSearchAdapter(url, apiKey string) (*SearchAdapter, error) {
	client := ms.New(url, ms.WithAPIKey(apiKey))
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return newSearchAdapterFromClient(ctx, client)
}

func newSearchAdapterFromClient(ctx context.Context, client ServiceClient) (*SearchAdapter, error) {
	if _, err := client.GetIndexWithContext(ctx, indexName); err != nil {
		if _, createErr := client.CreateIndexWithContext(ctx, &ms.IndexConfig{
			Uid:        indexName,
			PrimaryKey: "id",
		}); createErr != nil {
			return nil, fmt.Errorf("create index: %w", createErr)
		}
	}

	idx := client.Index(indexName)

	if _, err := idx.UpdateSettingsWithContext(ctx, &ms.Settings{
		SearchableAttributes: []string{"title", "original_title", "translated_titles", "cast"},
		FilterableAttributes: []string{"type", "library_id", "year", "genres"},
		SortableAttributes:   []string{"title", "year", "added_at"},
		RankingRules: []string{
			"words", "typo", "proximity", "attribute", "sort", "exactness",
		},
		Synonyms: map[string][]string{
			"film":   {"movie", "movies"},
			"movie":  {"film", "films"},
			"serie":  {"series", "show", "tv show"},
			"series": {"serie", "show", "tv show"},
			"show":   {"serie", "series", "tv show"},
		},
	}); err != nil {
		slog.Warn("update meilisearch settings", slog.Any("err", err))
	}

	return &SearchAdapter{index: idx}, nil
}

type mediaDocument struct {
	ID               string   `json:"id"`
	Title            string   `json:"title"`
	OriginalTitle    string   `json:"original_title"`
	TranslatedTitles []string `json:"translated_titles"`
	Genres           []string `json:"genres"`
	Cast             []string `json:"cast"`
	Year             *int     `json:"year"`
	Type             string   `json:"type"`
	LibraryID        string   `json:"library_id"`
	AddedAt          int64    `json:"added_at"`
	// Translations maps a normalized locale ("fr", "es", …) to the item's title in
	// that locale. Stored (retrievable) but NOT searchable - recall is already
	// covered by translated_titles. It feeds the profile-language re-rank: among
	// comparable Meili hits, a French viewer's French-title match is floated above
	// one that only matched a foreign title or cast. See reRankByProfileLocale.
	Translations map[string]string `json:"translations"`
}

func ToDocument(item *library.MediaItem) mediaDocument {
	doc := mediaDocument{
		ID:               string(item.ID),
		Title:            item.Title,
		OriginalTitle:    item.OriginalTitle,
		TranslatedTitles: translatedTitles(item),
		Translations:     translationsByLocale(item),
		Cast:             item.CastNames,
		Type:             string(item.Type),
		LibraryID:        string(item.LibraryID),
		AddedAt:          item.AddedAt.Unix(),
	}
	if item.Catalog != nil {
		doc.Genres = item.Catalog.Genres
		doc.Year = item.Catalog.Year
	}
	return doc
}

// translatedTitles collects the distinct per-locale titles carried on the item,
// skipping empties and any that duplicate the primary/original title (those are
// already searchable). Lets a user find a title only stored in a translation.
func translatedTitles(item *library.MediaItem) []string {
	if len(item.Translations) == 0 {
		return nil
	}
	seen := map[string]bool{
		strings.ToLower(strings.TrimSpace(item.Title)):         true,
		strings.ToLower(strings.TrimSpace(item.OriginalTitle)): true,
	}
	var out []string
	for _, tr := range item.Translations {
		t := strings.TrimSpace(tr.Title)
		if t == "" {
			continue
		}
		key := strings.ToLower(t)
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, t)
	}
	return out
}

// translationsByLocale maps each normalized locale to the item's title in that
// locale, for the profile-language re-rank. Keyed by normalizeLocale so a stored
// "fr-FR"/"fr_FR"/"fr" all resolve under "fr" (the form LocaleFromCtx yields).
// First non-empty title per normalized locale wins. nil when no translations.
func translationsByLocale(item *library.MediaItem) map[string]string {
	if len(item.Translations) == 0 {
		return nil
	}
	out := make(map[string]string, len(item.Translations))
	for _, tr := range item.Translations {
		title := strings.TrimSpace(tr.Title)
		if title == "" {
			continue
		}
		key := normalizeLocale(tr.Locale)
		if key == "" {
			continue
		}
		if _, exists := out[key]; !exists {
			out[key] = title
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// normalizeLocale reduces a locale tag to its lowercase primary subtag:
// "fr-FR"/"fr_FR"/"FR" → "fr". Aligns the stored translation keys with the
// short code LocaleFromCtx returns so the re-rank lookup hits.
func normalizeLocale(s string) string {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "_", "-")
	if i := strings.IndexByte(s, '-'); i >= 0 {
		s = s[:i]
	}
	return s
}

// normalizeForMatch lowercases and collapses whitespace for the re-rank's
// substring test. Best-effort (no diacritic folding): Meili already handled
// recall accent-insensitively, so a miss here just leaves the hit in Meili order
// rather than dropping it.
func normalizeForMatch(s string) string {
	return strings.Join(strings.Fields(strings.ToLower(s)), " ")
}

// reRankByProfileLocale stable-partitions hits so those whose title IN THE
// VIEWER'S LOCALE contains the query come first, preserving Meili's relative
// order within each group. A tie-break boost layered on top of Meili relevance,
// not a re-sort: Meilisearch has no per-query field weighting, so this is how a
// French viewer's French-title match outranks a hit that only matched via a
// foreign translation or cast. localeTitles[i] is the locale title for hits[i]
// ("" when the item has none). Caller skips this for the fallback locale.
func reRankByProfileLocale(hits []library.SearchHit, localeTitles []string, query string) []library.SearchHit {
	q := normalizeForMatch(query)
	if q == "" {
		return hits
	}
	matched := make([]library.SearchHit, 0, len(hits))
	rest := make([]library.SearchHit, 0, len(hits))
	for i, h := range hits {
		if i < len(localeTitles) && localeTitles[i] != "" && strings.Contains(normalizeForMatch(localeTitles[i]), q) {
			matched = append(matched, h)
		} else {
			rest = append(rest, h)
		}
	}
	if len(matched) == 0 {
		return hits // nothing to float - keep Meili's order untouched
	}
	return append(matched, rest...)
}

var allowedFilterKeys = map[string]bool{
	"type": true, "library_id": true, "year": true, "genres": true,
	"year_min": true, "year_max": true,
}

func BuildFilterString(filters map[string]string) string {
	if len(filters) == 0 {
		return ""
	}
	parts := make([]string, 0, len(filters))
	for k, v := range filters {
		if !allowedFilterKeys[k] {
			continue
		}
		switch {
		case strings.HasSuffix(k, "_min"):
			field := strings.TrimSuffix(k, "_min")
			parts = append(parts, fmt.Sprintf("%s >= %s", field, sanitizeNumeric(v)))
		case strings.HasSuffix(k, "_max"):
			field := strings.TrimSuffix(k, "_max")
			parts = append(parts, fmt.Sprintf("%s <= %s", field, sanitizeNumeric(v)))
		default:
			escaped := strings.ReplaceAll(v, "\\", "\\\\")
			escaped = strings.ReplaceAll(escaped, "'", "\\'")
			parts = append(parts, fmt.Sprintf("%s = '%s'", k, escaped))
		}
	}
	return strings.Join(parts, " AND ")
}

func sanitizeNumeric(v string) string {
	v = strings.TrimSpace(v)
	for _, c := range v {
		if (c < '0' || c > '9') && c != '-' {
			return "0"
		}
	}
	if v == "" || v == "-" {
		return "0"
	}
	return v
}

func (a *SearchAdapter) HealthCheck(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.HealthCheck")
	defer span.End()
	_, err := a.index.GetStatsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("meilisearch health: %w", err)
	}
	return nil
}

func (a *SearchAdapter) DocumentCount(ctx context.Context) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.DocumentCount")
	defer span.End()
	stats, err := a.index.GetStatsWithContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("get index stats: %w", err)
	}
	return stats.NumberOfDocuments, nil
}

func (a *SearchAdapter) IndexMediaItem(ctx context.Context, item *library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.IndexMediaItem")
	defer span.End()
	doc := ToDocument(item)
	_, err := a.index.AddDocumentsWithContext(ctx, []mediaDocument{doc}, nil)
	if err != nil {
		return fmt.Errorf("index media item: %w", err)
	}
	return nil
}

func (a *SearchAdapter) IndexMediaItemsBatch(ctx context.Context, items []*library.MediaItem) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.IndexMediaItemsBatch")
	defer span.End()
	if len(items) == 0 {
		return nil
	}
	docs := make([]mediaDocument, len(items))
	for i, item := range items {
		docs[i] = ToDocument(item)
	}
	_, err := a.index.AddDocumentsWithContext(ctx, docs, nil)
	if err != nil {
		return fmt.Errorf("index media items batch: %w", err)
	}
	return nil
}

func (a *SearchAdapter) RemoveMediaItem(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.RemoveMediaItem")
	defer span.End()
	_, err := a.index.DeleteDocumentWithContext(ctx, id, nil)
	if err != nil {
		return fmt.Errorf("remove media item: %w", err)
	}
	return nil
}

// Clear deletes every document in the index. Used by the reconcile to evict
// stale docs (a delete dropped during an outage) before a full reindex - an
// upsert reindex alone can never shrink the index back to the DB count.
func (a *SearchAdapter) Clear(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.Clear")
	defer span.End()
	if _, err := a.index.DeleteAllDocumentsWithContext(ctx, nil); err != nil {
		return fmt.Errorf("clear index: %w", err)
	}
	return nil
}

func (a *SearchAdapter) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.Search")
	defer span.End()
	if limit <= 0 {
		limit = 20
	}
	// The profile locale rides the request context (set at the gRPC boundary).
	// Only retrieve+re-rank on translations for a NON-fallback locale: an English
	// (fallback) viewer's canonical titles are already the search language, so the
	// path stays byte-identical to before for them.
	profileLocale := normalizeLocale(eveyi18n.LocaleFromCtx(ctx))
	rerank := profileLocale != "" && profileLocale != eveyi18n.FallbackLocale

	retrieve := []string{"id", "library_id", "title", "year", "type", "genres"}
	if rerank {
		retrieve = append(retrieve, "translations")
	}
	req := &ms.SearchRequest{
		Limit:                 limit,
		AttributesToRetrieve:  retrieve,
		AttributesToHighlight: []string{"title"},
		HighlightPreTag:       "<mark>",
		HighlightPostTag:      "</mark>",
		MatchingStrategy:      ms.Frequency,
	}
	if f := BuildFilterString(filters); f != "" {
		req.Filter = f
	}
	if len(sort) > 0 {
		req.Sort = sort
	}

	resp, err := a.index.SearchWithContext(ctx, query, req)
	if err != nil {
		return nil, 0, fmt.Errorf("search: %w", err)
	}

	hits := make([]library.SearchHit, 0, len(resp.Hits))
	var localeTitles []string
	if rerank {
		localeTitles = make([]string, 0, len(resp.Hits))
	}
	for _, hit := range resp.Hits {
		var doc struct {
			ID             string            `json:"id"`
			LibraryID      string            `json:"library_id"`
			Title          string            `json:"title"`
			Year           *int              `json:"year"`
			Type           string            `json:"type"`
			Genres         []string          `json:"genres"`
			Tags           []string          `json:"tags"`
			OfficialRating string            `json:"official_rating"`
			Translations   map[string]string `json:"translations"`
		}
		if err := hit.DecodeInto(&doc); err != nil {
			continue
		}
		h := library.SearchHit{
			ID:             doc.ID,
			LibraryID:      shared.LibraryID(doc.LibraryID),
			Title:          doc.Title,
			Year:           doc.Year,
			Type:           doc.Type,
			Genres:         doc.Genres,
			Tags:           doc.Tags,
			OfficialRating: doc.OfficialRating,
		}
		// The localized title is what the library grid shows for this same media
		// (overlayMediaItemsLocale), so search must show it too - a viewer who
		// reads one title in the grid and another in the results is looking at
		// two answers to one question. Free here: the translations are already
		// retrieved and decoded for the re-rank below; they were simply thrown
		// away afterwards. An absent translation keeps the stored title.
		if rerank {
			if lt := doc.Translations[profileLocale]; lt != "" {
				h.Title = lt
			}
		}
		if raw, ok := hit["_formatted"]; ok {
			var fm map[string]string
			if json.Unmarshal(raw, &fm) == nil {
				if ht, ok := fm["title"]; ok {
					h.HighlightedTitle = ht
				}
			}
		}
		hits = append(hits, h)
		if rerank {
			localeTitles = append(localeTitles, doc.Translations[profileLocale])
		}
	}

	if rerank {
		hits = reRankByProfileLocale(hits, localeTitles, query)
	}

	totalHits := resp.EstimatedTotalHits
	return hits, totalHits, nil
}
