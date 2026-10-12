package shared_test

import (
	"errors"
	"math"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
)

func TestNewDurationMs_Valid(t *testing.T) {
	t.Parallel()
	cases := []int64{0, 1, 1000, math.MaxInt64}
	for _, ms := range cases {
		d, err := shared.NewDurationMs(ms)
		if err != nil {
			t.Errorf("NewDurationMs(%d) unexpected error: %v", ms, err)
		}
		if d.Milliseconds() != ms {
			t.Errorf("NewDurationMs(%d).Milliseconds() = %d", ms, d.Milliseconds())
		}
	}
}

func TestNewDurationMs_Negative(t *testing.T) {
	t.Parallel()
	_, err := shared.NewDurationMs(-1)
	if !errors.Is(err, shared.ErrNegativeDuration) {
		t.Fatalf("expected ErrNegativeDuration, got %v", err)
	}
	_, err = shared.NewDurationMs(math.MinInt64)
	if !errors.Is(err, shared.ErrNegativeDuration) {
		t.Fatalf("expected ErrNegativeDuration for MinInt64, got %v", err)
	}
}

func TestNewDurationFromTime_Rounding(t *testing.T) {
	t.Parallel()
	// Convention: math.Round (half away from zero). 1.5ms → 2ms, 1.4ms → 1ms,
	// 1.6ms → 2ms. Documents the choice via assertion.
	cases := []struct {
		in   time.Duration
		want int64
	}{
		{0, 0},
		{time.Millisecond, 1},
		{1500 * time.Microsecond, 2}, // 1.5 ms → 2 ms
		{1400 * time.Microsecond, 1}, // 1.4 ms → 1 ms
		{1600 * time.Microsecond, 2}, // 1.6 ms → 2 ms
		{499 * time.Microsecond, 0},  // 0.499 ms → 0 ms
		{500 * time.Microsecond, 1},  // 0.5 ms → 1 ms (away from zero)
		{time.Second, 1000},
	}
	for _, c := range cases {
		d, err := shared.NewDurationFromTime(c.in)
		if err != nil {
			t.Errorf("NewDurationFromTime(%v) unexpected error: %v", c.in, err)
			continue
		}
		if d.Milliseconds() != c.want {
			t.Errorf("NewDurationFromTime(%v).Milliseconds() = %d, want %d", c.in, d.Milliseconds(), c.want)
		}
	}
}

func TestNewDurationFromTime_Negative(t *testing.T) {
	t.Parallel()
	_, err := shared.NewDurationFromTime(-time.Second)
	if !errors.Is(err, shared.ErrNegativeDuration) {
		t.Fatalf("expected ErrNegativeDuration, got %v", err)
	}
}

func TestDuration_AccessorsAndZero(t *testing.T) {
	t.Parallel()
	zero, _ := shared.NewDurationMs(0)
	if !zero.IsZero() {
		t.Error("0 ms duration must report IsZero")
	}
	if zero.Milliseconds() != 0 || zero.Seconds() != 0 || zero.Time() != 0 {
		t.Error("0 ms duration accessors must all return 0")
	}

	d, _ := shared.NewDurationMs(2500)
	if d.IsZero() {
		t.Error("2500 ms duration must not report IsZero")
	}
	if d.Milliseconds() != 2500 {
		t.Errorf("Milliseconds() = %d, want 2500", d.Milliseconds())
	}
	if d.Seconds() != 2.5 {
		t.Errorf("Seconds() = %v, want 2.5", d.Seconds())
	}
	if d.Time() != 2500*time.Millisecond {
		t.Errorf("Time() = %v, want 2.5s", d.Time())
	}
}

func TestDuration_Equal(t *testing.T) {
	t.Parallel()
	a, _ := shared.NewDurationMs(1000)
	b, _ := shared.NewDurationMs(1000)
	c, _ := shared.NewDurationMs(2000)
	if !a.Equal(b) {
		t.Error("a.Equal(b) must be true for equal ms")
	}
	if a.Equal(c) {
		t.Error("a.Equal(c) must be false for different ms")
	}
	zero, _ := shared.NewDurationMs(0)
	if !zero.Equal(shared.Duration{}) {
		t.Error("explicit zero must Equal the zero value")
	}
}

func TestDuration_Less(t *testing.T) {
	t.Parallel()
	a, _ := shared.NewDurationMs(1000)
	b, _ := shared.NewDurationMs(2000)
	c, _ := shared.NewDurationMs(1000)
	if !a.Less(b) {
		t.Error("a < b must be true")
	}
	if b.Less(a) {
		t.Error("b < a must be false")
	}
	if a.Less(c) {
		t.Error("a < c (equal) must be false")
	}
}

func TestDuration_Add(t *testing.T) {
	t.Parallel()
	a, _ := shared.NewDurationMs(1000)
	b, _ := shared.NewDurationMs(500)
	sum, err := a.Add(b)
	if err != nil {
		t.Fatalf("Add unexpected error: %v", err)
	}
	if sum.Milliseconds() != 1500 {
		t.Errorf("a.Add(b) = %d ms, want 1500", sum.Milliseconds())
	}

	// Identity with zero
	zero, _ := shared.NewDurationMs(0)
	sum, err = a.Add(zero)
	if err != nil || !sum.Equal(a) {
		t.Errorf("a.Add(0) = (%v, %v), want (a, nil)", sum, err)
	}
}

func TestDuration_AddOverflow(t *testing.T) {
	t.Parallel()
	max, _ := shared.NewDurationMs(math.MaxInt64)
	one, _ := shared.NewDurationMs(1)
	if _, err := max.Add(one); !errors.Is(err, shared.ErrDurationOverflow) {
		t.Fatalf("expected ErrDurationOverflow, got %v", err)
	}
	// Boundary: max + 0 must succeed
	zero, _ := shared.NewDurationMs(0)
	if _, err := max.Add(zero); err != nil {
		t.Fatalf("max + 0 must not overflow, got %v", err)
	}
}

func TestDuration_String(t *testing.T) {
	t.Parallel()
	// 1h23m45.123s = 3600000 + 23*60*1000 + 45*1000 + 123 = 5025123 ms
	d, err := shared.NewDurationMs(5025123)
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	got := d.String()
	want := "1h23m45.123s"
	if got != want {
		t.Errorf("String() = %q, want %q", got, want)
	}

	zero, _ := shared.NewDurationMs(0)
	if zero.String() != "0s" {
		t.Errorf("zero.String() = %q, want %q", zero.String(), "0s")
	}
}
