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

// And conjoins specs; the empty And is always satisfied (identity).
func And(specs ...Spec) Spec { return AndSpec{Specs: specs} }

// AndSpec and the specs below are exported so a storage adapter can translate
// them to its own query language; the domain keeps their in-memory semantics.
type AndSpec struct{ Specs []Spec }

func (a AndSpec) IsSatisfiedBy(c Candidate) bool {
	for _, s := range a.Specs {
		if !s.IsSatisfiedBy(c) {
			return false
		}
	}
	return true
}

type GenreSpec struct{ Genre string }

func (g GenreSpec) IsSatisfiedBy(c Candidate) bool {
	for _, gr := range c.Genres {
		if strings.EqualFold(gr, g.Genre) {
			return true
		}
	}
	return false
}

type TypeSpec struct{ MediaType string }

func (t TypeSpec) IsSatisfiedBy(c Candidate) bool { return c.Type == t.MediaType }

type YearOp string

const (
	YearEq  YearOp = "eq"
	YearGt  YearOp = "gt"
	YearLt  YearOp = "lt"
	YearGte YearOp = "gte"
	YearLte YearOp = "lte"
)

type YearSpec struct {
	Op   YearOp
	Year int
}

func (y YearSpec) IsSatisfiedBy(c Candidate) bool {
	if c.Year == nil {
		return false
	}
	v := *c.Year
	switch y.Op {
	case YearGt:
		return v > y.Year
	case YearLt:
		return v < y.Year
	case YearGte:
		return v >= y.Year
	case YearLte:
		return v <= y.Year
	default:
		return v == y.Year
	}
}

type LibraryIDSpec struct{ ID string }

func (l LibraryIDSpec) IsSatisfiedBy(c Candidate) bool { return c.LibraryID == l.ID }

type RatingGteSpec struct{ Min float64 }

func (r RatingGteSpec) IsSatisfiedBy(c Candidate) bool {
	if c.CommunityRating == nil {
		return false
	}
	return *c.CommunityRating >= r.Min
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
		return GenreSpec{Genre: r.Value}, nil
	case "type":
		if r.Value == "" {
			return nil, nil
		}
		return TypeSpec{MediaType: r.Value}, nil
	case "year":
		y, err := strconv.Atoi(r.Value)
		if err != nil {
			return nil, fmt.Errorf("year rule: %w", err)
		}
		op := YearOp(r.Operator)
		switch op {
		case YearEq, YearGt, YearLt, YearGte, YearLte, "":
			if op == "" {
				op = YearEq
			}
			return YearSpec{Op: op, Year: y}, nil
		default:
			return nil, fmt.Errorf("year rule: invalid operator %q", r.Operator)
		}
	case "library_id":
		if r.Value == "" {
			return nil, nil
		}
		return LibraryIDSpec{ID: r.Value}, nil
	case "rating_gte":
		f, err := strconv.ParseFloat(r.Value, 64)
		if err != nil {
			return nil, fmt.Errorf("rating_gte rule: %w", err)
		}
		return RatingGteSpec{Min: f}, nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownRuleField, r.Field)
	}
}
