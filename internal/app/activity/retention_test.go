package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/activity"
	activityfakes "github.com/eveyproject/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
)

// runRetention DELETES rows. Every branch in it was unmeasured until a first
// mutation pass on internal/app/activity (2026-08-08) came back at 49% efficacy
// - the weakest package measured in this repository - with twelve of the
// survivors sitting right here.
//
// That matters more than the number: a negated guard on this path either purges
// history the operator asked to keep, or silently keeps growing a table that was
// supposed to be bounded. Neither shows up as an error anywhere. These tests pin
// the four decisions the function actually makes.

type purgeSpy struct {
	calls  []time.Time
	err    error
	panics bool
	n      int64
}

func (p *purgeSpy) Purge(_ context.Context, before time.Time) (int64, error) {
	if p.panics {
		panic("purge exploded")
	}
	p.calls = append(p.calls, before)
	return p.n, p.err
}

func newRetentionService(t *testing.T) (*CommandService, *activityfakes.FakeAuditLogRepository, *activityfakes.FakeServerMetricRepository) {
	t.Helper()
	audit := activityfakes.NewFakeAuditLogRepository()
	metrics := activityfakes.NewFakeServerMetricRepository()
	svc := NewCommandService(
		activityfakes.NewFakeLoginHistoryRepository(),
		metrics,
		audit,
		sharedfakes.NewFakeEventBus(),
	)
	return svc, audit, metrics
}

// Retention disabled must mean "delete nothing". A guard flipped the wrong way
// here is data loss the operator explicitly opted out of.
func TestRunRetention_ZeroDaysPurgesNothing(t *testing.T) {
	svc, _, _ := newRetentionService(t)
	webhooks, qoe := &purgeSpy{}, &purgeSpy{}
	svc.WithWebhookDeliveryPurger(webhooks).WithQoEEventPurger(qoe)

	svc.runRetention(context.Background(), 0, 0, 0)

	if len(qoe.calls) != 0 {
		t.Errorf("qoe purge ran with retention disabled: %v", qoe.calls)
	}
	// Webhook deliveries are the deliberate exception: they fall back to their
	// own default rather than being kept forever, because unlike the audit log
	// they carry no compliance argument.
	if len(webhooks.calls) != 1 {
		t.Fatalf("webhook purge calls = %d, want 1 (it falls back to its own default)", len(webhooks.calls))
	}
}

func TestRunRetention_CutoffIsNowMinusRetention(t *testing.T) {
	svc, _, _ := newRetentionService(t)
	qoe := &purgeSpy{}
	svc.WithQoEEventPurger(qoe)

	before := time.Now()
	svc.runRetention(context.Background(), 0, 0, 7)
	after := time.Now()

	if len(qoe.calls) != 1 {
		t.Fatalf("qoe purge calls = %d, want 1", len(qoe.calls))
	}
	got := qoe.calls[0]
	// The cutoff must land 7 days back, not 7 days forward and not today -
	// a sign error here deletes everything the retention was meant to keep.
	if got.After(after.AddDate(0, 0, -7)) || got.Before(before.AddDate(0, 0, -7).Add(-time.Second)) {
		t.Errorf("qoe cutoff = %v, want ~now-7d (between %v and %v)",
			got, before.AddDate(0, 0, -7), after.AddDate(0, 0, -7))
	}
}

// The webhook window rides the audit window, and falls back to its own default
// only when audit retention is off.
func TestRunRetention_WebhookWindowFollowsAuditThenFallsBack(t *testing.T) {
	t.Run("follows the audit window when it is set", func(t *testing.T) {
		svc, _, _ := newRetentionService(t)
		webhooks := &purgeSpy{}
		svc.WithWebhookDeliveryPurger(webhooks)

		svc.runRetention(context.Background(), 3, 0, 0)

		if len(webhooks.calls) != 1 {
			t.Fatalf("webhook purge calls = %d, want 1", len(webhooks.calls))
		}
		wantAbout := time.Now().AddDate(0, 0, -3)
		if webhooks.calls[0].Sub(wantAbout) > time.Minute || wantAbout.Sub(webhooks.calls[0]) > time.Minute {
			t.Errorf("webhook cutoff = %v, want ~now-3d (%v)", webhooks.calls[0], wantAbout)
		}
	})

	t.Run("falls back to its own default when audit retention is off", func(t *testing.T) {
		svc, _, _ := newRetentionService(t)
		webhooks := &purgeSpy{}
		svc.WithWebhookDeliveryPurger(webhooks)

		svc.runRetention(context.Background(), 0, 0, 0)

		if len(webhooks.calls) != 1 {
			t.Fatalf("webhook purge calls = %d, want 1", len(webhooks.calls))
		}
		wantAbout := time.Now().AddDate(0, 0, -defaultWebhookDeliveryRetentionDays)
		if webhooks.calls[0].Sub(wantAbout) > time.Minute || wantAbout.Sub(webhooks.calls[0]) > time.Minute {
			t.Errorf("webhook cutoff = %v, want ~now-%dd (%v)",
				webhooks.calls[0], defaultWebhookDeliveryRetentionDays, wantAbout)
		}
	})
}

// The optional purgers are optional: an unwired one must be skipped, not
// dereferenced. This runs in a detached daily goroutine, so a nil panic here
// would take the process down and stop every future purge.
func TestRunRetention_UnwiredPurgersAreSkipped(t *testing.T) {
	svc, _, _ := newRetentionService(t)

	svc.runRetention(context.Background(), 5, 5, 5) // no purgers wired at all
}

// A failing purge must not stop the ones after it, and a panicking one must not
// escape: the caller is a detached goroutine with no recovery of its own.
func TestRunRetention_OneFailureDoesNotStopTheRest(t *testing.T) {
	svc, _, _ := newRetentionService(t)
	webhooks := &purgeSpy{err: errors.New("db down")}
	qoe := &purgeSpy{}
	svc.WithWebhookDeliveryPurger(webhooks).WithQoEEventPurger(qoe)

	svc.runRetention(context.Background(), 1, 1, 1)

	if len(qoe.calls) != 1 {
		t.Errorf("a failing webhook purge stopped the qoe purge: qoe calls = %d, want 1", len(qoe.calls))
	}
}

func TestRunRetention_PanicIsContained(t *testing.T) {
	svc, _, _ := newRetentionService(t)
	svc.WithWebhookDeliveryPurger(&purgeSpy{panics: true})

	// The assertion is simply that this returns: an escaping panic would fail
	// the test by killing the run.
	svc.runRetention(context.Background(), 1, 0, 0)
}

// The two tests above assert that a purge RAN, which turned out to be a much
// weaker claim than it reads as: a mutation run on this package (2026-08-08)
// showed the audit and metric guards surviving every mutant, while the webhook
// and qoe ones died. The difference was not the code, it was that nothing here
// looked at the rows. Seeding entries of a known age and checking which ones
// are still standing afterwards is what separates `> 0` from `>= 0`, and
// `now-days` from `now+days` - both of which delete history that was meant to
// be kept, silently, on a goroutine nobody watches.

func seedAudit(t *testing.T, r *activityfakes.FakeAuditLogRepository, ages ...time.Duration) {
	t.Helper()
	for i, age := range ages {
		if err := r.Create(context.Background(), &activity.AuditLogEntry{
			ID:        string(rune('a' + i)),
			Action:    "some.action",
			CreatedAt: time.Now().Add(-age),
		}); err != nil {
			t.Fatalf("seed audit entry: %v", err)
		}
	}
}

func seedMetrics(t *testing.T, r *activityfakes.FakeServerMetricRepository, ages ...time.Duration) {
	t.Helper()
	for i, age := range ages {
		if err := r.Create(context.Background(), &activity.ServerMetric{
			ID:         string(rune('a' + i)),
			RecordedAt: time.Now().Add(-age),
		}); err != nil {
			t.Fatalf("seed metric: %v", err)
		}
	}
}

func countAudit(t *testing.T, r *activityfakes.FakeAuditLogRepository) int {
	t.Helper()
	n, err := r.Count(context.Background(), activity.AuditLogFilter{})
	if err != nil {
		t.Fatalf("count audit entries: %v", err)
	}
	return n
}

func countMetrics(t *testing.T, r *activityfakes.FakeServerMetricRepository) int {
	t.Helper()
	got, err := r.GetByTimeRange(context.Background(), time.Now().AddDate(-50, 0, 0), time.Now().AddDate(50, 0, 0))
	if err != nil {
		t.Fatalf("read metrics back: %v", err)
	}
	return len(got)
}

func TestRunRetention_DropsRowsOlderThanTheWindowAndKeepsTheRest(t *testing.T) {
	svc, audit, metrics := newRetentionService(t)
	day := 24 * time.Hour
	seedAudit(t, audit, 10*day, 3*day)
	seedMetrics(t, metrics, 10*day, 3*day)

	svc.runRetention(context.Background(), 7, 7, 0)

	// Exactly the ten-day-old row goes. Keeping both means the purge never ran;
	// keeping neither means the cutoff landed in the future.
	if got := countAudit(t, audit); got != 1 {
		t.Errorf("audit entries left = %d, want 1 (the 3-day-old one, with a 7-day window)", got)
	}
	if got := countMetrics(t, metrics); got != 1 {
		t.Errorf("metrics left = %d, want 1 (the 3-day-old one, with a 7-day window)", got)
	}
}

func TestRunRetention_DisabledKeepsEvenTheOldestRows(t *testing.T) {
	svc, audit, metrics := newRetentionService(t)
	seedAudit(t, audit, 3650*24*time.Hour)
	seedMetrics(t, metrics, 3650*24*time.Hour)

	svc.runRetention(context.Background(), 0, 0, 0)

	// Ten years old and retention switched off: this row must still be there.
	// A guard flipped to `>= 0` purges with a cutoff of now, which deletes the
	// entire table precisely when the operator asked to keep it forever.
	if got := countAudit(t, audit); got != 1 {
		t.Errorf("audit entries left = %d, want 1 - retention is disabled, nothing may be deleted", got)
	}
	if got := countMetrics(t, metrics); got != 1 {
		t.Errorf("metrics left = %d, want 1 - retention is disabled, nothing may be deleted", got)
	}
}

var _ activity.QoEEventPurger = (*purgeSpy)(nil)
var _ activity.WebhookDeliveryPurger = (*purgeSpy)(nil)
