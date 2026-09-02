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

	// maxCandidatePool bounds the random track sample the generator ranks, so a
	// large library isn't loaded whole into memory. Far above maxGenSize so the
	// scorer still has a deep reservoir to pick the top tracks from.
	maxCandidatePool = 5000

	wGenreAffinity = 0.2  // per play accrued in the candidate's genre (taste affinity)
	wPlayCount     = 1.0  // per play of the candidate track itself (favourite-ish)
	dupPenalty     = 0.15 // multiplier for tracks already in one of the profile's playlists (soft, not excluded)
	jitterFloor    = 0.5  // score *= jitterFloor + rand[0,1) → variety + the no-taste fallback (pure random)
)

// GenTrackLister supplies the candidate pool. SampleByLibrary returns up to
// limit RANDOM tracks so a huge library isn't loaded whole into memory just to
// pick a few dozen - genre affinity is computed separately (from the profile's
// full history), so the sample only needs to be a deep-enough reservoir to rank.
type GenTrackLister interface {
	SampleByLibrary(ctx context.Context, libraryID shared.LibraryID, limit int) ([]music.Track, error)
}

// GenPlayCountGetter reads the profile's music taste signals (music tracks
// aren't media_items, so favorites/ratings can't apply): per-track play counts
// and per-genre affinity, both from the full play history.
type GenPlayCountGetter interface {
	PlayCountsForProfile(ctx context.Context, profileID string) (map[string]int, error)
	// GenreAffinityForProfile returns per-genre total plays from the full play
	// history - accurate independent of the (sampled) candidate pool.
	GenreAffinityForProfile(ctx context.Context, profileID string) (map[string]float64, error)
}

// GenPlaylistStore is the narrow slice of the playlist repo the generator needs:
// create the new playlist, list the profile's playlists for de-duplication, and
// delete it again to roll back a generation that fails partway through.
type GenPlaylistStore interface {
	Create(ctx context.Context, pl *playlist.Playlist) error
	List(ctx context.Context, profileID string) ([]*playlist.Playlist, error)
	Delete(ctx context.Context, id string) error
}

// GenItemStore is the narrow slice of the playlist-item repo the generator needs.
type GenItemStore interface {
	Add(ctx context.Context, item *playlist.PlaylistItem) error
	// GetByPlaylistIDs batches the de-duplication lookup across every profile
	// playlist into one call instead of one per playlist.
	GetByPlaylistIDs(ctx context.Context, playlistIDs []string) (map[string][]*playlist.PlaylistItem, error)
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

	// An empty name means the caller had no opinion, so ours may be adjusted to
	// get out of its own way. A name the person typed is theirs and is never
	// silently changed.
	explicitName := strings.TrimSpace(name) != ""
	if !explicitName {
		name = DefaultGeneratedName
	}

	if size <= 0 {
		size = defaultGenSize
	}
	if size > maxGenSize {
		size = maxGenSize
	}

	pool, err := g.tracks.SampleByLibrary(ctx, libraryID, maxCandidatePool)
	if err != nil {
		return nil, fmt.Errorf("sample tracks: %w", err)
	}
	if len(pool) == 0 {
		return nil, ErrNoTracks
	}

	playCounts := g.playCounts(ctx, profileID)
	inPlaylist := g.alreadyInPlaylists(ctx, profileID)
	genrePlays := g.genreAffinity(ctx, profileID)

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
	//nolint:gosec // playlist variety, not security - same rationale as the jitter above
	rand.Shuffle(len(candidates), func(i, j int) { candidates[i], candidates[j] = candidates[j], candidates[i] })

	pl, err := g.createNamed(ctx, profileID, name, explicitName)
	if err != nil {
		return nil, err
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
			// Don't leave a half-populated playlist behind and report success.
			// There's no unit-of-work here, so compensate by deleting the
			// just-created playlist (its items cascade) and surfacing the error.
			if delErr := g.playlists.Delete(ctx, pl.ID); delErr != nil {
				slog.ErrorContext(ctx, "generate playlist: rollback after add failure",
					slog.String("playlist_id", pl.ID), slog.Any("err", delErr))
			}
			return nil, fmt.Errorf("add playlist item: %w", err)
		}
	}
	return pl, nil
}

func (g *GeneratorService) playCounts(ctx context.Context, profileID string) map[string]int {
	counts, err := g.plays.PlayCountsForProfile(ctx, profileID)
	if err != nil {
		slog.WarnContext(ctx, "generate playlist: play counts", slog.Any("err", err))
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
		slog.WarnContext(ctx, "generate playlist: list profile playlists", slog.Any("err", err))
		return set
	}
	if len(pls) == 0 {
		return set
	}
	ids := make([]string, len(pls))
	for i, p := range pls {
		ids[i] = p.ID
	}
	byPlaylist, err := g.items.GetByPlaylistIDs(ctx, ids)
	if err != nil {
		slog.WarnContext(ctx, "generate playlist: batch lookup playlist items", slog.Any("err", err))
		return set
	}
	for _, items := range byPlaylist {
		for _, it := range items {
			set[string(it.MediaItemID)] = true
		}
	}
	return set
}

// genreAffinity returns per-genre play weights for the profile, favouring the
// genres it actually listens to. Computed from the full play history in the repo
// (independent of the sampled candidate pool). Best-effort: a lookup failure
// degrades the mix to play-count + jitter rather than failing the generation.
func (g *GeneratorService) genreAffinity(ctx context.Context, profileID string) map[string]float64 {
	aff, err := g.plays.GenreAffinityForProfile(ctx, profileID)
	if err != nil {
		slog.WarnContext(ctx, "generate playlist: genre affinity", slog.Any("err", err))
		return nil
	}
	return aff
}

// DefaultGeneratedName is what a generated playlist is called when the person
// did not name it.
const DefaultGeneratedName = "Mix"

// maxNameAttempts bounds the search for a free name. A profile with sixty
// mixes has a naming problem this code cannot solve for them; past that the
// collision is reported like any other.
const maxNameAttempts = 50

// createNamed persists the playlist, working around a name collision ONLY when
// the name is ours to choose.
//
// Prod, 2026-09-02: generating a second mix returned a 500, because "Mix" was
// already taken by the first and one profile may hold one playlist per name.
// Pressing the button twice is the most ordinary thing a person does, so the
// default name now steps aside; an explicit one is reported so the person can
// decide what to do about it.
func (g *GeneratorService) createNamed(ctx context.Context, profileID, name string, explicit bool) (*playlist.Playlist, error) {
	for attempt := range maxNameAttempts {
		candidate := name
		if attempt > 0 {
			candidate = fmt.Sprintf("%s %d", name, attempt+1)
		}
		pl, err := playlist.NewPlaylist(candidate, profileID, playlist.Manual)
		if err != nil {
			return nil, fmt.Errorf("new playlist: %w", err)
		}
		createErr := g.playlists.Create(ctx, pl)
		if createErr == nil {
			return pl, nil
		}
		if !errors.Is(createErr, playlist.ErrDuplicateName) || explicit {
			return nil, fmt.Errorf("create playlist: %w", createErr)
		}
	}
	return nil, fmt.Errorf("create playlist: %w", playlist.ErrDuplicateName)
}
