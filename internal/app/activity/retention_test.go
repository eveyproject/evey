package activity

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/evey-project/evey/internal/domain/activity"
	activityfakes "github.com/evey-project/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
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

var _ activity.QoEEventPurger = (*purgeSpy)(nil)
var _ activity.WebhookDeliveryPurger = (*purgeSpy)(nil)
