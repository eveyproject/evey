package grpcapi

import "testing"

func TestContainsStr_FoundAndMissing(t *testing.T) {
	t.Parallel()
	if !containsStr([]string{"a", "b", "c"}, "b") {
		t.Fatal("want true for present element")
	}
	if containsStr([]string{"a", "b", "c"}, "z") {
		t.Fatal("want false for missing element")
	}
	if containsStr(nil, "x") {
		t.Fatal("nil slice contains nothing")
	}
	if containsStr([]string{}, "x") {
		t.Fatal("empty slice contains nothing")
	}
}
