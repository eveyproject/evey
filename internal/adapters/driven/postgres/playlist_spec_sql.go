package postgres

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/eveyproject/evey/internal/domain/playlist"
)

// playlistSpecSQL compiles a smart playlist spec to a WHERE fragment over the
// mediaItemFromJoins aliases. argIdx is the next positional placeholder and is
// advanced by the call; an empty clause contributes nothing (neutral element).
func playlistSpecSQL(spec playlist.Spec, argIdx *int) (string, []any) {
	next := func() int { n := *argIdx; *argIdx++; return n }
	switch s := spec.(type) {
	case playlist.AndSpec:
		var parts []string
		var args []any
		for _, sub := range s.Specs {
			clause, cargs := playlistSpecSQL(sub, argIdx)
			if clause == "" {
				continue
			}
			parts = append(parts, clause)
			args = append(args, cargs...)
		}
		return strings.Join(parts, " AND "), args
	case playlist.GenreSpec:
		// json.Marshal, not hand quoting: a backslash or a control character in
		// a genre made the jsonb literal invalid and the whole query fail.
		arr, _ := json.Marshal([]string{s.Genre})
		return fmt.Sprintf("mc.genres @> $%d::jsonb", next()), []any{string(arr)}
	case playlist.TypeSpec:
		return fmt.Sprintf("mi.type = $%d", next()), []any{s.MediaType}
	case playlist.YearSpec:
		op := "="
		switch s.Op {
		case playlist.YearGt:
			op = ">"
		case playlist.YearLt:
			op = "<"
		case playlist.YearGte:
			op = ">="
		case playlist.YearLte:
			op = "<="
		}
		return fmt.Sprintf("mc.year %s $%d", op, next()), []any{s.Year}
	case playlist.LibraryIDSpec:
		return fmt.Sprintf("mi.library_id = $%d", next()), []any{s.ID}
	case playlist.RatingGteSpec:
		return fmt.Sprintf("mcl.community_rating >= $%d::double precision", next()), []any{s.Min}
	default:
		return "", nil
	}
}
