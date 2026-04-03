package rating

import "fmt"

// RatingValue is a validated 1-10 rating score.
type RatingValue struct {
	value int
}

const (
	minRatingValue = 1
	maxRatingValue = 10
)

// NewRatingValue validates and constructs a RatingValue (1-10)
func NewRatingValue(v int) (RatingValue, error) {
	if v < minRatingValue || v > maxRatingValue {
		return RatingValue{}, fmt.Errorf("rating must be between %d and %d, got %d", minRatingValue, maxRatingValue, v)
	}
	return RatingValue{value: v}, nil
}

func (r RatingValue) Int() int { return r.value }
