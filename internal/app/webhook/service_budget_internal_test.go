package webhook

import (
	"testing"
	"time"
)

// An event delivery must outlive its own retry schedule: with a 30 s budget
// and 1+5+30 s of backoff the last retry was never sent.
func TestAsyncDeliveryBudget_CoversEveryAttempt(t *testing.T) {
	t.Parallel()
	s := &Service{retryDelays: defaultRetryDelays}
	need := time.Duration(len(defaultRetryDelays)+1) * attemptTimeout
	for _, d := range defaultRetryDelays {
		need += d
	}
	if got := s.asyncDeliveryBudget(); got < need {
		t.Fatalf("budget %s < %s of backoff plus attempts", got, need)
	}
}
