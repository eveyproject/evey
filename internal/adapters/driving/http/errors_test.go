package httpapi

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

// A client that goes away cancels the context, and every in-flight read then
// fails with that cancellation. Reported as ERROR it is indistinguishable from
// a real fault, and it dominates the stream: in prod, EVERY http-side ERROR of
// the last three days was a client abort. The classifier is what keeps them
// apart, so it is worth pinning.
func TestIsClientCanceled(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"bare cancel", context.Canceled, true},
		{"wrapped cancel", fmt.Errorf("scanRows: %w", context.Canceled), true},
		{"deadline", context.DeadlineExceeded, true},
		{"pq cancellation without wrapping", errors.New(`pq: canceling statement due to user request (SQLSTATE 57014)`), true},
		{"real failure", errors.New("connection refused"), false},
		{"real failure that merely mentions a number", errors.New("read 57013 bytes"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isClientCanceled(tc.err); got != tc.want {
				t.Errorf("isClientCanceled(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}
