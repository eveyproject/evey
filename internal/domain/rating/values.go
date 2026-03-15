package rating

import (
	"database/sql/driver"
	"fmt"
)

// RatingValue is a validated 1-10 rating score.
// Mixed receivers: Scan needs pointer (sql.Scanner), Value needs value (driver.Valuer).
type RatingValue struct { //nolint:recvcheck
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

// Value implements driver.Valuer for database serialization.
func (r RatingValue) Value() (driver.Value, error) { return int64(r.value), nil }

// Scan implements sql.Scanner for database deserialization.
func (r *RatingValue) Scan(src any) error {
	var v int64
	switch t := src.(type) {
	case int64:
		v = t
	case []byte:
		_, err := fmt.Sscan(string(t), &v)
		if err != nil {
			return fmt.Errorf("RatingValue.Scan: %w", err)
		}
	default:
		return fmt.Errorf("RatingValue.Scan: unsupported type %T", src)
	}
	rv, err := NewRatingValue(int(v))
	if err != nil {
		return err
	}
	*r = rv
	return nil
}
