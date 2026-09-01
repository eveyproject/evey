package meilisearch

import (
	"context"
	"fmt"
	"log/slog"

	ms "github.com/meilisearch/meilisearch-go"

	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

const musicIndexName = "music_tracks"

// MusicSearchAdapter indexes music tracks in a dedicated Meilisearch index.
// Unlike the lightweight media hits, the document carries the full track so a
// search result is directly playable (file_path, duration, …) without a second
// DB round-trip.
type MusicSearchAdapter struct {
	index IndexClient
}

func NewMusicSearchAdapter(url, apiKey string) (*MusicSearchAdapter, error) {
	client := ms.New(url, ms.WithAPIKey(apiKey))
	ctx, cancel := context.WithTimeout(context.Background(), connectTimeout)
	defer cancel()
	return newMusicSearchAdapterFromClient(ctx, client)
}

func newMusicSearchAdapterFromClient(ctx context.Context, client ServiceClient) (*MusicSearchAdapter, error) {
	if _, err := client.GetIndexWithContext(ctx, musicIndexName); err != nil {
		if _, createErr := client.CreateIndexWithContext(ctx, &ms.IndexConfig{
			Uid:        musicIndexName,
			PrimaryKey: "id",
		}); createErr != nil {
			return nil, fmt.Errorf("create music index: %w", createErr)
		}
	}

	idx := client.Index(musicIndexName)

	if _, err := idx.UpdateSettingsWithContext(ctx, &ms.Settings{
		SearchableAttributes: []string{"title", "artist", "album_artist", "album"},
		FilterableAttributes: []string{"library_id"},
		RankingRules: []string{
			"words", "typo", "proximity", "attribute", "exactness",
		},
	}); err != nil {
		slog.Warn("update meilisearch music settings", slog.Any("err", err))
	}

	return &MusicSearchAdapter{index: idx}, nil
}

// musicDocument is the full denormalized track stored in Meilisearch. It mirrors
// music.Track so a hit reconstructs a playable track with no DB hydration.
type musicDocument struct {
	ID          string  `json:"id"`
	LibraryID   string  `json:"library_id"`
	Title       string  `json:"title"`
	Artist      string  `json:"artist"`
	Album       string  `json:"album"`
	AlbumArtist string  `json:"album_artist"`
	Year        int     `json:"year"`
	TrackNumber int     `json:"track_number"`
	DiskNumber  int     `json:"disk_number"`
	Duration    float64 `json:"duration"`
	FilePath    string  `json:"file_path"`
	FileSize    int64   `json:"file_size"`
	MimeType    string  `json:"mime_type"`
	Bitrate     int     `json:"bitrate"`
	SampleRate  int     `json:"sample_rate"`
	Channels    int     `json:"channels"`
	Genre       string  `json:"genre"`
	AlbumFolder string  `json:"album_folder"`
	CoverArtURL string  `json:"cover_art_url"`
}

func musicToDocument(t music.Track) musicDocument {
	return musicDocument{
		ID:          t.ID,
		LibraryID:   string(t.LibraryID),
		Title:       t.Title,
		Artist:      t.Artist,
		Album:       t.Album,
		AlbumArtist: t.AlbumArtist,
		Year:        t.Year,
		TrackNumber: t.TrackNumber,
		DiskNumber:  t.DiskNumber,
		Duration:    t.Duration,
		FilePath:    t.FilePath,
		FileSize:    t.FileSize,
		MimeType:    t.MimeType,
		Bitrate:     t.Bitrate,
		SampleRate:  t.SampleRate,
		Channels:    t.Channels,
		Genre:       t.Genre,
		AlbumFolder: t.AlbumFolder,
		CoverArtURL: t.CoverArtURL,
	}
}

func (d musicDocument) toTrack() music.Track {
	return music.Track{
		ID:          d.ID,
		LibraryID:   shared.LibraryID(d.LibraryID),
		Title:       d.Title,
		Artist:      d.Artist,
		Album:       d.Album,
		AlbumArtist: d.AlbumArtist,
		Year:        d.Year,
		TrackNumber: d.TrackNumber,
		DiskNumber:  d.DiskNumber,
		Duration:    d.Duration,
		FilePath:    d.FilePath,
		FileSize:    d.FileSize,
		MimeType:    d.MimeType,
		Bitrate:     d.Bitrate,
		SampleRate:  d.SampleRate,
		Channels:    d.Channels,
		Genre:       d.Genre,
		AlbumFolder: d.AlbumFolder,
		CoverArtURL: d.CoverArtURL,
	}
}

func (a *MusicSearchAdapter) HealthCheck(ctx context.Context) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.music.HealthCheck")
	defer span.End()
	if _, err := a.index.GetStatsWithContext(ctx, nil); err != nil {
		return fmt.Errorf("meilisearch music health: %w", err)
	}
	return nil
}

func (a *MusicSearchAdapter) DocumentCount(ctx context.Context) (int64, error) {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.music.DocumentCount")
	defer span.End()
	stats, err := a.index.GetStatsWithContext(ctx, nil)
	if err != nil {
		return 0, fmt.Errorf("get music index stats: %w", err)
	}
	return stats.NumberOfDocuments, nil
}

func (a *MusicSearchAdapter) IndexTrack(ctx context.Context, t music.Track) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.music.IndexTrack")
	defer span.End()
	if _, err := a.index.AddDocumentsWithContext(ctx, []musicDocument{musicToDocument(t)}, nil); err != nil {
		return fmt.Errorf("index track: %w", err)
	}
	return nil
}

func (a *MusicSearchAdapter) IndexTracksBatch(ctx context.Context, tracks []music.Track) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.music.IndexTracksBatch")
	defer span.End()
	if len(tracks) == 0 {
		return nil
	}
	docs := make([]musicDocument, len(tracks))
	for i, t := range tracks {
		docs[i] = musicToDocument(t)
	}
	if _, err := a.index.AddDocumentsWithContext(ctx, docs, nil); err != nil {
		return fmt.Errorf("index tracks batch: %w", err)
	}
	return nil
}

func (a *MusicSearchAdapter) RemoveTrack(ctx context.Context, id string) error {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.music.RemoveTrack")
	defer span.End()
	if _, err := a.index.DeleteDocumentWithContext(ctx, id, nil); err != nil {
		return fmt.Errorf("remove track: %w", err)
	}
	return nil
}

func (a *MusicSearchAdapter) Search(ctx context.Context, libraryID shared.LibraryID, query string, limit int) ([]music.Track, error) {
	ctx, span := tracing.StartSpan(ctx, "meilisearch.music.Search")
	defer span.End()
	if limit <= 0 {
		limit = 20
	}
	req := &ms.SearchRequest{
		Limit:            int64(limit),
		MatchingStrategy: ms.Frequency,
	}
	// Empty libraryID = global search; only scope when a library is given. The
	// value is interpolated into a Meili filter, so escape quotes defensively
	// (library ids are server-issued slugs, but never trust the boundary).
	if libraryID != "" {
		req.Filter = BuildFilterString(map[string]string{"library_id": string(libraryID)})
	}
	resp, err := a.index.SearchWithContext(ctx, query, req)
	if err != nil {
		return nil, fmt.Errorf("music search: %w", err)
	}
	tracks := make([]music.Track, 0, len(resp.Hits))
	for _, hit := range resp.Hits {
		var doc musicDocument
		if err := hit.DecodeInto(&doc); err != nil {
			continue
		}
		tracks = append(tracks, doc.toTrack())
	}
	return tracks, nil
}
