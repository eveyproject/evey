package feed

import "testing"

// Pin exact itoa output to kill arithmetic and boundary mutations in the digit loop.
func TestItoa_ExactOutput(t *testing.T) {
	cases := []struct {
		in   int
		want string
	}{
		{0, "0"},
		{1, "1"},
		{9, "9"},
		{10, "10"},
		{12, "12"},
		{13, "13"},
		{21, "21"},
		{99, "99"},
		{123, "123"},
		{1000, "1000"},
		{-1, "-1"},
		{-5, "-5"},
		{-10, "-10"},
		{-123, "-123"},
	}
	for _, tc := range cases {
		got := itoa(tc.in)
		if got != tc.want {
			t.Errorf("itoa(%d) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
