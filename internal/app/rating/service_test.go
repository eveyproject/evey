package rating_test

import (
	"context"
	"testing"

	apprating "github.com/gravity-zero/evey/internal/app/rating"
	sharedfakes "github.com/gravity-zero/evey/internal/domain/shared/fakes"
	"github.com/gravity-zero/evey/internal/domain/rating/fakes"
)

func newSvc(t *testing.T) *apprating.Service {
	t.Helper()
	return apprating.NewService(fakes.NewFakeRepository(), sharedfakes.NewFakeEventBus())
}

func TestRate_ValidValues(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	for _, v := range []int{1, 2, 3, 4, 5} {
		if err := svc.Rate(ctx, "media-1", "profile-1", v); err != nil {
			t.Fatalf("Rate(%d): %v", v, err)
		}
	}
}

func TestRate_InvalidValues(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	for _, v := range []int{0, 11, -1} {
		if err := svc.Rate(ctx, "media-1", "profile-1", v); err == nil {
			t.Fatalf("Rate(%d) expected error, got nil", v)
		}
	}
}

func TestGetRating_NotFound(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	rt, err := svc.GetRating(ctx, "media-1", "profile-1")
	if err != nil {
		t.Fatal(err)
	}
	if rt != nil {
		t.Fatalf("expected nil, got %+v", rt)
	}
}

func TestRateAndGet(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	if err := svc.Rate(ctx, "media-1", "profile-1", 4); err != nil {
		t.Fatal(err)
	}
	rt, err := svc.GetRating(ctx, "media-1", "profile-1")
	if err != nil || rt == nil || rt.Value.Int() != 4 {
		t.Fatalf("got %+v, err %v", rt, err)
	}
}

func TestDeleteRating(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	svc.Rate(ctx, "media-1", "profile-1", 3)
	svc.DeleteRating(ctx, "media-1", "profile-1")
	rt, err := svc.GetRating(ctx, "media-1", "profile-1")
	if err != nil || rt != nil {
		t.Fatalf("expected nil after delete, got %+v, err %v", rt, err)
	}
}

func TestGetAggregates(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	svc.Rate(ctx, "media-1", "profile-1", 4)
	svc.Rate(ctx, "media-1", "profile-2", 2)

	aggs, err := svc.GetAggregates(ctx, []string{"media-1", "media-2"})
	if err != nil {
		t.Fatal(err)
	}
	agg := aggs["media-1"]
	if agg.Count != 2 {
		t.Fatalf("expected count 2, got %d", agg.Count)
	}
	if agg.Average != 3.0 {
		t.Fatalf("expected average 3.0, got %f", agg.Average)
	}
	if _, ok := aggs["media-2"]; ok {
		t.Fatal("media-2 should have no aggregate")
	}
}

func TestRate_Upsert(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	if err := svc.Rate(ctx, "media-1", "profile-1", 3); err != nil {
		t.Fatal(err)
	}
	if err := svc.Rate(ctx, "media-1", "profile-1", 5); err != nil {
		t.Fatalf("second Rate (upsert) expected no error: %v", err)
	}
	rt, err := svc.GetRating(ctx, "media-1", "profile-1")
	if err != nil || rt == nil || rt.Value.Int() != 5 {
		t.Fatalf("expected value 5 after upsert, got %+v, err %v", rt, err)
	}
}

func TestRate_PublishesEvent(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo, bus)
	ctx := context.Background()

	if err := svc.Rate(ctx, "media-1", "profile-1", 3); err != nil {
		t.Fatal(err)
	}
	events := bus.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].EventName() != "rating.media.rated" {
		t.Errorf("unexpected event name %q", events[0].EventName())
	}
}

func TestDeleteRating_PublishesEvent(t *testing.T) {
	bus := sharedfakes.NewFakeEventBus()
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo, bus)
	ctx := context.Background()

	svc.Rate(ctx, "media-1", "profile-1", 3)
	bus2 := sharedfakes.NewFakeEventBus()
	svc2 := apprating.NewService(repo, bus2)
	if err := svc2.DeleteRating(ctx, "media-1", "profile-1"); err != nil {
		t.Fatal(err)
	}
	events := bus2.PublishedEvents()
	if len(events) != 1 {
		t.Fatalf("expected 1 event after delete, got %d", len(events))
	}
	if events[0].EventName() != "rating.media.deleted" {
		t.Errorf("unexpected event name %q", events[0].EventName())
	}
}

func TestGetAggregates_MultipleItems(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	svc.Rate(ctx, "media-1", "profile-1", 5)
	svc.Rate(ctx, "media-1", "profile-2", 3)
	svc.Rate(ctx, "media-2", "profile-1", 2)

	aggs, err := svc.GetAggregates(ctx, []string{"media-1", "media-2", "media-3"})
	if err != nil {
		t.Fatal(err)
	}
	if aggs["media-1"].Count != 2 {
		t.Fatalf("media-1 count: expected 2, got %d", aggs["media-1"].Count)
	}
	if aggs["media-1"].Average != 4.0 {
		t.Fatalf("media-1 avg: expected 4.0, got %f", aggs["media-1"].Average)
	}
	if aggs["media-2"].Count != 1 {
		t.Fatalf("media-2 count: expected 1, got %d", aggs["media-2"].Count)
	}
	if _, ok := aggs["media-3"]; ok {
		t.Fatal("media-3 should have no aggregate")
	}
}

func TestGetRating_AfterUpsert(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	svc.Rate(ctx, "media-X", "profile-A", 1)
	svc.Rate(ctx, "media-X", "profile-A", 5)

	rt, err := svc.GetRating(ctx, "media-X", "profile-A")
	if err != nil {
		t.Fatal(err)
	}
	if rt == nil || rt.Value.Int() != 5 {
		t.Fatalf("expected value 5, got %+v", rt)
	}
}

func TestGetRating_IsolatedByProfile(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	svc.Rate(ctx, "media-1", "profile-A", 2)
	svc.Rate(ctx, "media-1", "profile-B", 4)

	rtA, err := svc.GetRating(ctx, "media-1", "profile-A")
	if err != nil || rtA == nil || rtA.Value.Int() != 2 {
		t.Fatalf("profile-A expected 2, got %+v err %v", rtA, err)
	}
	rtB, err := svc.GetRating(ctx, "media-1", "profile-B")
	if err != nil || rtB == nil || rtB.Value.Int() != 4 {
		t.Fatalf("profile-B expected 4, got %+v err %v", rtB, err)
	}
}

func TestGetAggregates_EmptySlice(t *testing.T) {
	svc := newSvc(t)
	ctx := context.Background()

	aggs, err := svc.GetAggregates(ctx, []string{})
	if err != nil {
		t.Fatal(err)
	}
	if len(aggs) != 0 {
		t.Fatalf("expected empty map, got %d entries", len(aggs))
	}
}
