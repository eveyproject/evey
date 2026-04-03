package rating

import "testing"

func TestNewRatingValue_Valid(t *testing.T) {
	t.Parallel()
	for _, v := range []int{1, 2, 5, 9, 10} {
		rv, err := NewRatingValue(v)
		if err != nil {
			t.Fatalf("NewRatingValue(%d): %v", v, err)
		}
		if rv.Int() != v {
			t.Fatalf("Int() = %d, want %d", rv.Int(), v)
		}
	}
}

func TestNewRatingValue_Invalid(t *testing.T) {
	t.Parallel()
	for _, v := range []int{0, -1, -100, 11, 100} {
		_, err := NewRatingValue(v)
		if err == nil {
			t.Fatalf("NewRatingValue(%d) expected error", v)
		}
	}
}
