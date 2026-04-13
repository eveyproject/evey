package meilisearch

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	ms "github.com/meilisearch/meilisearch-go"

	"github.com/gravity-zero/evey/internal/domain/library"
)

const (
	indexName      = "media_items"
	connectTimeout = 30 * time.Second
)

type IndexClient interface {
	AddDocumentsWithContext(ctx context.Context, documentsPtr any, opts *ms.DocumentOptions) (*ms.TaskInfo, error)
	DeleteDocumentWithContext(ctx context.Context, identifier string, opts *ms.DocumentOptions) (*ms.TaskInfo, error)
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
		SearchableAttributes: []string{"title", "original_title", "cast"},
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
	ID            string   `json:"id"`
	Title         string   `json:"title"`
	OriginalTitle string   `json:"original_title"`
	Genres        []string `json:"genres"`
	Cast          []string `json:"cast"`
	Year          *int     `json:"year"`
	Type          string   `json:"type"`
	LibraryID     string   `json:"library_id"`
	AddedAt       int64    `json:"added_at"`
}

func ToDocument(item *library.MediaItem, castNames []string) mediaDocument {
	return mediaDocument{
		ID:            item.ID,
		Title:         item.Title,
		OriginalTitle: item.OriginalTitle,
		Genres:        item.Genres,
		Cast:          castNames,
		Year:          item.Year,
		Type:          string(item.Type),
		LibraryID:     item.LibraryID,
		AddedAt:       item.AddedAt.Unix(),
	}
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
	_, err := a.index.GetStatsWithContext(ctx)
	if err != nil {
		return fmt.Errorf("meilisearch health: %w", err)
	}
	return nil
}

func (a *SearchAdapter) DocumentCount(ctx context.Context) (int64, error) {
	stats, err := a.index.GetStatsWithContext(ctx)
	if err != nil {
		return 0, fmt.Errorf("get index stats: %w", err)
	}
	return stats.NumberOfDocuments, nil
}

func (a *SearchAdapter) IndexMediaItem(ctx context.Context, item *library.MediaItem, castNames []string) error {
	doc := ToDocument(item, castNames)
	_, err := a.index.AddDocumentsWithContext(ctx, []mediaDocument{doc}, nil)
	if err != nil {
		return fmt.Errorf("index media item: %w", err)
	}
	return nil
}

func (a *SearchAdapter) IndexMediaItemsBatch(ctx context.Context, items []*library.MediaItem) error {
	if len(items) == 0 {
		return nil
	}
	docs := make([]mediaDocument, len(items))
	for i, item := range items {
		docs[i] = ToDocument(item, nil)
	}
	_, err := a.index.AddDocumentsWithContext(ctx, docs, nil)
	if err != nil {
		return fmt.Errorf("index media items batch: %w", err)
	}
	return nil
}

func (a *SearchAdapter) RemoveMediaItem(ctx context.Context, id string) error {
	_, err := a.index.DeleteDocumentWithContext(ctx, id, nil)
	if err != nil {
		return fmt.Errorf("remove media item: %w", err)
	}
	return nil
}

func (a *SearchAdapter) Search(ctx context.Context, query string, filters map[string]string, sort []string, limit int64) ([]library.SearchHit, int64, error) {
	if limit <= 0 {
		limit = 20
	}
	req := &ms.SearchRequest{
		Limit:                 limit,
		AttributesToRetrieve:  []string{"id", "library_id", "title", "year", "type", "genres"},
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
	for _, hit := range resp.Hits {
		var doc struct {
			ID             string   `json:"id"`
			LibraryID      string   `json:"library_id"`
			Title          string   `json:"title"`
			Year           *int     `json:"year"`
			Type           string   `json:"type"`
			Genres         []string `json:"genres"`
			Tags           []string `json:"tags"`
			OfficialRating string   `json:"official_rating"`
		}
		if err := hit.DecodeInto(&doc); err != nil {
			continue
		}
		h := library.SearchHit{
			ID:             doc.ID,
			LibraryID:      doc.LibraryID,
			Title:          doc.Title,
			Year:           doc.Year,
			Type:           doc.Type,
			Genres:         doc.Genres,
			Tags:           doc.Tags,
			OfficialRating: doc.OfficialRating,
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
	}

	totalHits := resp.EstimatedTotalHits
	return hits, totalHits, nil
}
