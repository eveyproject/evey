package playlist

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

// Candidate is the minimal projection of a media item needed to evaluate
// playlist rules. Keeping it in the playlist domain avoids pulling
// library.MediaItem into a cross-domain import.
type Candidate struct {
	ID              shared.MediaItemID
	Type            string
	Genres          []string
	Year            *int
	CommunityRating *float64
	LibraryID       string
	SortTitle       string
	AddedAt         time.Time
}

// Spec is a predicate on a Candidate.
type Spec interface {
	IsSatisfiedBy(c Candidate) bool
}

// SQLSpec is an optional capability: specs that can compile themselves to
// a SQL WHERE fragment using the shared mediaItem query joins.
// argIdx is the next positional placeholder number and is advanced by the
// call; when clause is empty the spec contributes nothing (neutral element).
type SQLSpec interface {
	ToSQL(argIdx *int) (clause string, args []any)
}

// And conjoins specs; the empty And is always satisfied (identity).
func And(specs ...Spec) Spec { return andSpec{specs: specs} }

type andSpec struct{ specs []Spec }

func (a andSpec) IsSatisfiedBy(c Candidate) bool {
	for _, s := range a.specs {
		if !s.IsSatisfiedBy(c) {
			return false
		}
	}
	return true
}

func (a andSpec) ToSQL(argIdx *int) (string, []any) {
	var parts []string
	var args []any
	for _, s := range a.specs {
		sq, ok := s.(SQLSpec)
		if !ok {
			continue
		}
		clause, cargs := sq.ToSQL(argIdx)
		if clause == "" {
			continue
		}
		parts = append(parts, clause)
		args = append(args, cargs...)
	}
	return strings.Join(parts, " AND "), args
}

type genreSpec struct{ genre string }

func (g genreSpec) IsSatisfiedBy(c Candidate) bool {
	for _, gr := range c.Genres {
		if strings.EqualFold(gr, g.genre) {
			return true
		}
	}
	return false
}

func (g genreSpec) ToSQL(argIdx *int) (string, []any) {
	clause := fmt.Sprintf("mc.genres @> $%d::jsonb", *argIdx)
	*argIdx++
	return clause, []any{fmt.Sprintf(`["%s"]`, strings.ReplaceAll(g.genre, `"`, `\"`))}
}

type typeSpec struct{ mediaType string }

func (t typeSpec) IsSatisfiedBy(c Candidate) bool { return c.Type == t.mediaType }

func (t typeSpec) ToSQL(argIdx *int) (string, []any) {
	clause := fmt.Sprintf("mi.type = $%d", *argIdx)
	*argIdx++
	return clause, []any{t.mediaType}
}

type yearOp string

const (
	yearEq  yearOp = "eq"
	yearGt  yearOp = "gt"
	yearLt  yearOp = "lt"
	yearGte yearOp = "gte"
	yearLte yearOp = "lte"
)

type yearSpec struct {
	op   yearOp
	year int
}

func (y yearSpec) IsSatisfiedBy(c Candidate) bool {
	if c.Year == nil {
		return false
	}
	v := *c.Year
	switch y.op {
	case yearGt:
		return v > y.year
	case yearLt:
		return v < y.year
	case yearGte:
		return v >= y.year
	case yearLte:
		return v <= y.year
	default:
		return v == y.year
	}
}

func (y yearSpec) ToSQL(argIdx *int) (string, []any) {
	var op string
	switch y.op {
	case yearGt:
		op = ">"
	case yearLt:
		op = "<"
	case yearGte:
		op = ">="
	case yearLte:
		op = "<="
	default:
		op = "="
	}
	clause := fmt.Sprintf("mc.year %s $%d", op, *argIdx)
	*argIdx++
	return clause, []any{y.year}
}

type libraryIDSpec struct{ id string }

func (l libraryIDSpec) IsSatisfiedBy(c Candidate) bool { return c.LibraryID == l.id }

func (l libraryIDSpec) ToSQL(argIdx *int) (string, []any) {
	clause := fmt.Sprintf("mi.library_id = $%d", *argIdx)
	*argIdx++
	return clause, []any{l.id}
}

type ratingGteSpec struct{ min float64 }

func (r ratingGteSpec) IsSatisfiedBy(c Candidate) bool {
	if c.CommunityRating == nil {
		return false
	}
	return *c.CommunityRating >= r.min
}

func (r ratingGteSpec) ToSQL(argIdx *int) (string, []any) {
	clause := fmt.Sprintf("mcl.community_rating >= $%d::double precision", *argIdx)
	*argIdx++
	return clause, []any{r.min}
}

// ErrUnknownRuleField is returned when CompileRules encounters a field it
// can't map to a known spec type.
var ErrUnknownRuleField = errors.New("unknown rule field")

// CompileRules parses persisted Rules into a Spec tree (AND-joined).
// Unknown fields are surfaced as ErrUnknownRuleField so admin UIs can flag
// bad input instead of silently returning empty playlists.
func CompileRules(rules []Rule) (Spec, error) {
	if len(rules) == 0 {
		return And(), nil
	}
	specs := make([]Spec, 0, len(rules))
	for _, r := range rules {
		s, err := compileRule(r)
		if err != nil {
			return nil, err
		}
		if s == nil {
			continue
		}
		specs = append(specs, s)
	}
	return And(specs...), nil
}

func compileRule(r Rule) (Spec, error) {
	switch r.Field {
	case "genre":
		if r.Value == "" {
			return nil, nil
		}
		return genreSpec{genre: r.Value}, nil
	case "type":
		if r.Value == "" {
			return nil, nil
		}
		return typeSpec{mediaType: r.Value}, nil
	case "year":
		y, err := strconv.Atoi(r.Value)
		if err != nil {
			return nil, fmt.Errorf("year rule: %w", err)
		}
		op := yearOp(r.Operator)
		switch op {
		case yearEq, yearGt, yearLt, yearGte, yearLte, "":
			if op == "" {
				op = yearEq
			}
			return yearSpec{op: op, year: y}, nil
		default:
			return nil, fmt.Errorf("year rule: invalid operator %q", r.Operator)
		}
	case "library_id":
		if r.Value == "" {
			return nil, nil
		}
		return libraryIDSpec{id: r.Value}, nil
	case "rating_gte":
		f, err := strconv.ParseFloat(r.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("rating_gte rule: %w", err)
		}
		return ratingGteSpec{min: f}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownRuleField, r.Field)
	}
}
