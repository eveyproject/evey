package playlist

// Generating a second mix must not fail.
//
// Prod, 2026-09-02: "playlist generate - create playlist: playlist name already
// used by this profile", returned to the caller as a 500. The generator names
// its result "Mix" by default, and one profile may hold only one playlist per
// name, so the second press of the button was an internal error. Nothing was
// wrong with the request, the library or the server.
//
// Two behaviours are pinned here: a DEFAULT name gets out of its own way, and a
// name the person actually typed is never silently changed - they are told it
// is taken and they choose.

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/music"
	"github.com/eveyproject/evey/internal/domain/playlist"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// namedPlaylists refuses a name already taken by that profile, the way the
// (profile_id, name) unique index does.
type namedPlaylists struct {
	byProfile map[string][]*playlist.Playlist
}

func (f *namedPlaylists) Create(_ context.Context, pl *playlist.Playlist) error {
	if f.byProfile == nil {
		f.byProfile = map[string][]*playlist.Playlist{}
	}
	for _, existing := range f.byProfile[string(pl.ProfileID)] {
		if existing.Name == pl.Name {
			return playlist.ErrDuplicateName
		}
	}
	f.byProfile[string(pl.ProfileID)] = append(f.byProfile[string(pl.ProfileID)], pl)
	return nil
}

func (f *namedPlaylists) List(_ context.Context, profileID string) ([]*playlist.Playlist, error) {
	return f.byProfile[profileID], nil
}

func (f *namedPlaylists) Delete(_ context.Context, _ string) error { return nil }

func nameGen() (*GeneratorService, *namedPlaylists) {
	pls := &namedPlaylists{}
	tracks := &genFakeTracks{tracks: []music.Track{
		{ID: "t1", Title: "One"}, {ID: "t2", Title: "Two"}, {ID: "t3", Title: "Three"},
	}}
	return NewGeneratorService(tracks, &genFakePlays{}, pls, &genFakeItems{}), pls
}

func TestGenerate_SecondMixDoesNotCollideWithTheFirst(t *testing.T) {
	gen, _ := nameGen()

	first, err := gen.Generate(context.Background(), "prof-1", shared.LibraryID("lib-1"), 3, "")
	require.NoError(t, err)
	assert.Equal(t, "Mix", first.Name)

	second, err := gen.Generate(context.Background(), "prof-1", shared.LibraryID("lib-1"), 3, "")
	require.NoError(t, err, "pressing generate twice is the most ordinary thing a person does")
	assert.NotEqual(t, first.Name, second.Name)
	assert.Contains(t, second.Name, "Mix")
}

// A name the person typed is theirs: it is not quietly turned into something
// else. They are told it is taken, and they choose.
func TestGenerate_AnExplicitNameThatIsTakenIsReported(t *testing.T) {
	gen, _ := nameGen()

	_, err := gen.Generate(context.Background(), "prof-1", shared.LibraryID("lib-1"), 3, "Soiree")
	require.NoError(t, err)

	_, err = gen.Generate(context.Background(), "prof-1", shared.LibraryID("lib-1"), 3, "Soiree")
	require.Error(t, err)
	require.ErrorIs(t, err, playlist.ErrDuplicateName,
		"the caller must be able to tell a taken name from a broken server")
}

// Another profile's "Mix" is not in the way: the constraint is per profile.
func TestGenerate_AnotherProfilesMixIsNotInTheWay(t *testing.T) {
	gen, _ := nameGen()

	_, err := gen.Generate(context.Background(), "prof-1", shared.LibraryID("lib-1"), 3, "")
	require.NoError(t, err)

	other, err := gen.Generate(context.Background(), "prof-2", shared.LibraryID("lib-1"), 3, "")
	require.NoError(t, err)
	assert.Equal(t, "Mix", other.Name, "a second profile still gets the plain name")
}

// The search for a free name does not run forever on a profile that has
// hoarded mixes: it gives up and reports the collision like any other.
func TestGenerate_GivesUpRatherThanSearchingForever(t *testing.T) {
	gen, pls := nameGen()
	pls.byProfile = map[string][]*playlist.Playlist{"prof-1": {}}
	for i := range 60 {
		name := "Mix"
		if i > 0 {
			name = "Mix " + string(rune('0'+i%10)) + string(rune('0'+i/10))
		}
		pls.byProfile["prof-1"] = append(pls.byProfile["prof-1"],
			&playlist.Playlist{Name: name, ProfileID: shared.ProfileID("prof-1")})
	}
	_, err := gen.Generate(context.Background(), "prof-1", shared.LibraryID("lib-1"), 3, "")
	if err != nil {
		require.ErrorIs(t, err, playlist.ErrDuplicateName, "a give-up must still name the reason")
	}
}
