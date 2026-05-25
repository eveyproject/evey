package playlist

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"sort"
	"strings"
	"time"

	"github.com/evey-project/evey/internal/domain/music"
	"github.com/evey-project/evey/internal/domain/playlist"
	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// ErrNoTracks is returned when the music library has nothing to generate from.
var ErrNoTracks = errors.New("no tracks available to generate a playlist")

// Generation tuning. Kept as named constants so the balance is easy to adjust.
const (
	defaultGenSize = 50
	maxGenSize     = 200 // stays well under the playlist item cap (500)

	wGenreAffinity = 0.2  // per play accrued in the candidate's genre (taste affinity)
	wPlayCount     = 1.0  // per play of the candidate track itself (favourite-ish)
	dupPenalty     = 0.15 // multiplier for tracks already in one of the profile's playlists (soft, not excluded)
	jitterFloor    = 0.5  // score *= jitterFloor + rand[0,1) → variety + the no-taste fallback (pure random)
)

// GenTrackLister lists every track of a music library (the candidate pool).
type GenTrackLister interface {
	ListAllByLibrary(ctx context.Context, libraryID shared.LibraryID) ([]music.Track, error)
}

// GenPlayCountGetter reads a profile's per-track play counts — the music taste
// signal (music tracks aren't media_items, so favorites/ratings can't apply).
type GenPlayCountGetter interface {
	PlayCountsForProfile(ctx context.Context, profileID string) (map[string]int, error)
}

// GenPlaylistStore is the narrow slice of the playlist repo the generator needs:
// create the new playlist, and list the profile's playlists for de-duplication.
type GenPlaylistStore interface {
	Create(ctx context.Context, pl *playlist.Playlist) error
	List(ctx context.Context, profileID string) ([]*playlist.Playlist, error)
}

// GenItemStore is the narrow slice of the playlist-item repo the generator needs.
type GenItemStore interface {
	Add(ctx context.Context, item *playlist.PlaylistItem) error
	GetByPlaylistID(ctx context.Context, playlistID string) ([]*playlist.PlaylistItem, error)
}

// GeneratorService builds a taste-weighted music playlist from a profile's
// listening history: it scores every track by how much the profile plays its
// genre and the track itself, soft-penalises tracks already in the profile's
// other playlists, and adds a random jitter (which also makes it degrade
// gracefully to a pure-random mix when there's no listening history yet), then
// persists the top picks as a manual playlist.
type GeneratorService struct {
	tracks    GenTrackLister
	plays     GenPlayCountGetter
	playlists GenPlaylistStore
	items     GenItemStore
}

func NewGeneratorService(tracks GenTrackLister, plays GenPlayCountGetter, playlists GenPlaylistStore, items GenItemStore) *GeneratorService {
	return &GeneratorService{tracks: tracks, plays: plays, playlists: playlists, items: items}
}

// Generate creates and persists a taste-weighted playlist of up to size tracks
// owned by the given profile. Taste signal and ownership are both per-profile.
func (g *GeneratorService) Generate(ctx context.Context, profileID string, libraryID shared.LibraryID, size int, name string) (*playlist.Playlist, error) {
	ctx, span := tracing.StartSpan(ctx, "app.playlist.Generate")
	defer span.End()

	if size <= 0 {
		size = defaultGenSize
	}
	if size > maxGenSize {
		size = maxGenSize
	}

	pool, err := g.tracks.ListAllByLibrary(ctx, libraryID)
	if err != nil {
		return nil, fmt.Errorf("list tracks: %w", err)
	}
	if len(pool) == 0 {
		return nil, ErrNoTracks
	}

	playCounts := g.playCounts(ctx, profileID)
	inPlaylist := g.alreadyInPlaylists(ctx, profileID)
	genrePlays := genreAffinity(pool, playCounts)

	type scored struct {
		track music.Track
		score float64
	}
	candidates := make([]scored, 0, len(pool))
	for _, t := range pool {
		score := 1.0
		if t.Genre != "" {
			score += genrePlays[strings.ToLower(t.Genre)] * wGenreAffinity
		}
		score += float64(playCounts[t.ID]) * wPlayCount
		if inPlaylist[t.ID] {
			score *= dupPenalty
		}
		score *= jitterFloor + rand.Float64() //nolint:gosec // playlist variety, not security
		candidates = append(candidates, scored{track: t, score: score})
	}

	sort.SliceStable(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	if len(candidates) > size {
		candidates = candidates[:size]
	}
	// Shuffle the selection so playback order isn't a giveaway of the scoring.
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	pl, err := playlist.NewPlaylist(name, profileID, playlist.Manual)
	if err != nil {
		return nil, fmt.Errorf("new playlist: %w", err)
	}
	if err := g.playlists.Create(ctx, pl); err != nil {
		return nil, fmt.Errorf("create playlist: %w", err)
	}
	now := time.Now()
	for i, c := range candidates {
		item := &playlist.PlaylistItem{
			PlaylistID:  pl.ID,
			MediaItemID: shared.MediaItemID(c.track.ID),
			SortOrder:   i,
			AddedAt:     now,
			MediaType:   playlist.MediaTypeMusic,
		}
		if err := g.items.Add(ctx, item); err != nil {
			slog.Warn("generate playlist: add item", slog.String("playlist_id", pl.ID), slog.String("track_id", c.track.ID), slog.Any("err", err))
		}
	}
	return pl, nil
}

func (g *GeneratorService) playCounts(ctx context.Context, profileID string) map[string]int {
	counts, err := g.plays.PlayCountsForProfile(ctx, profileID)
	if err != nil {
		slog.Warn("generate playlist: play counts", slog.Any("err", err))
		return nil
	}
	return counts
}

// alreadyInPlaylists returns the set of media item ids already present in any of
// the profile's playlists, for the soft de-duplication penalty.
func (g *GeneratorService) alreadyInPlaylists(ctx context.Context, profileID string) map[string]bool {
	set := make(map[string]bool)
	pls, err := g.playlists.List(ctx, profileID)
	if err != nil {
		slog.Warn("generate playlist: list profile playlists", slog.Any("err", err))
		return set
	}
	for _, p := range pls {
		items, itErr := g.items.GetByPlaylistID(ctx, p.ID)
		if itErr != nil {
			continue
		}
		for _, it := range items {
			set[string(it.MediaItemID)] = true
		}
	}
	return set
}

// genreAffinity weights each genre by the profile's total plays of tracks in
// that genre, so the scoring favours the genres the profile actually listens to.
func genreAffinity(pool []music.Track, playCounts map[string]int) map[string]float64 {
	weight := make(map[string]float64)
	for _, t := range pool {
		if c := playCounts[t.ID]; c > 0 && t.Genre != "" {
			weight[strings.ToLower(t.Genre)] += float64(c)
		}
	}
	return weight
}
