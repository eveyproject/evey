package postgres

import (
	"encoding/json"
	"testing"

	"github.com/eveyproject/evey/internal/domain/playlist"
)

func compiledSQL(t *testing.T, rules []playlist.Rule, argIdx *int) (string, []any) {
	t.Helper()
	spec, err := playlist.CompileRules(rules)
	if err != nil {
		t.Fatal(err)
	}
	return playlistSpecSQL(spec, argIdx)
}

func TestPlaylistSpecSQL_JoinsAndAdvancesPlaceholders(t *testing.T) {
	argIdx := 1
	clause, args := compiledSQL(t, []playlist.Rule{
		{Field: "type", Value: "movie"},
		{Field: "year", Operator: "gte", Value: "2020"},
		{Field: "library_id", Value: "lib-1"},
		{Field: "rating_gte", Value: "7.5"},
	}, &argIdx)
	want := "mi.type = $1 AND mc.year >= $2 AND mi.library_id = $3 AND mcl.community_rating >= $4::double precision"
	if clause != want {
		t.Fatalf("clause = %q, want %q", clause, want)
	}
	if len(args) != 4 || args[0] != "movie" || args[1] != 2020 || args[2] != "lib-1" || args[3] != 7.5 {
		t.Fatalf("args = %v", args)
	}
	if argIdx != 5 {
		t.Fatalf("argIdx = %d, want 5", argIdx)
	}
}

func TestPlaylistSpecSQL_EmptySpecIsNeutral(t *testing.T) {
	argIdx := 1
	if clause, args := compiledSQL(t, nil, &argIdx); clause != "" || len(args) != 0 || argIdx != 1 {
		t.Fatalf("empty spec = %q %v (argIdx %d), want no clause", clause, args, argIdx)
	}
}

// The genre is bound as a jsonb array literal: it must stay valid JSON for
// any genre text, or Postgres refuses the whole smart playlist query.
func TestPlaylistSpecSQL_GenreArgIsValidJSON(t *testing.T) {
	for _, genre := range []string{`Sci"Fi`, `Sci\`, "Tab\tGenre", `Back\"slash`} {
		argIdx := 1
		clause, args := compiledSQL(t, []playlist.Rule{{Field: "genre", Value: genre}}, &argIdx)
		if clause != "mc.genres @> $1::jsonb" {
			t.Fatalf("clause = %q", clause)
		}
		var got []string
		if err := json.Unmarshal([]byte(args[0].(string)), &got); err != nil || len(got) != 1 || got[0] != genre {
			t.Errorf("genre %q serialised as %q: %v", genre, args[0], err)
		}
	}
}
