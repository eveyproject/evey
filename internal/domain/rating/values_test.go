package rating

import (
	"database/sql/driver"
	"testing"
)

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

func TestRatingValue_Value(t *testing.T) {
	t.Parallel()
	rv, _ := NewRatingValue(7)
	got, err := rv.Value()
	if err != nil {
		t.Fatal(err)
	}
	if got != int64(7) {
		t.Fatalf("Value() = %v, want int64(7)", got)
	}
}

func TestRatingValue_Zero_Value(t *testing.T) {
	t.Parallel()
	var rv RatingValue
	got, err := rv.Value()
	if err != nil {
		t.Fatal(err)
	}
	if got != int64(0) {
		t.Fatalf("zero Value() = %v, want int64(0)", got)
	}
}

func TestRatingValue_Scan_Int64(t *testing.T) {
	t.Parallel()
	var rv RatingValue
	if err := rv.Scan(int64(5)); err != nil {
		t.Fatal(err)
	}
	if rv.Int() != 5 {
		t.Fatalf("after Scan(int64(5)), Int() = %d", rv.Int())
	}
}

func TestRatingValue_Scan_Bytes(t *testing.T) {
	t.Parallel()
	var rv RatingValue
	if err := rv.Scan([]byte("3")); err != nil {
		t.Fatal(err)
	}
	if rv.Int() != 3 {
		t.Fatalf("after Scan([]byte(\"3\")), Int() = %d", rv.Int())
	}
}

func TestRatingValue_Scan_InvalidBytes(t *testing.T) {
	t.Parallel()
	var rv RatingValue
	if err := rv.Scan([]byte("abc")); err == nil {
		t.Fatal("expected error for non-numeric bytes")
	}
}

func TestRatingValue_Scan_OutOfRange(t *testing.T) {
	t.Parallel()
	var rv RatingValue
	if err := rv.Scan(int64(0)); err == nil {
		t.Fatal("expected error for out-of-range value 0")
	}
	if err := rv.Scan(int64(11)); err == nil {
		t.Fatal("expected error for out-of-range value 11")
	}
}

func TestRatingValue_Scan_UnsupportedType(t *testing.T) {
	t.Parallel()
	var rv RatingValue
	if err := rv.Scan("string-not-bytes"); err == nil {
		t.Fatal("expected error for unsupported type string")
	}
}

func TestRatingValue_ImplementsDriverValuer(t *testing.T) {
	t.Parallel()
	rv, _ := NewRatingValue(1)
	var _ driver.Valuer = rv
}
