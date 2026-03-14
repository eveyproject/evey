package rating_test

import (
	"context"
	"testing"

	apprating "github.com/gravity-zero/evey/internal/app/rating"
	"github.com/gravity-zero/evey/internal/domain/rating/fakes"
)

func TestRate_ValidValues(t *testing.T) {
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo)
	ctx := context.Background()

	for _, v := range []int{1, 2, 3, 4, 5} {
		if err := svc.Rate(ctx, "media-1", "profile-1", v); err != nil {
			t.Fatalf("Rate(%d): %v", v, err)
		}
	}
}

func TestRate_InvalidValues(t *testing.T) {
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo)
	ctx := context.Background()

	for _, v := range []int{0, 6, -1} {
		if err := svc.Rate(ctx, "media-1", "profile-1", v); err == nil {
			t.Fatalf("Rate(%d) expected error, got nil", v)
		}
	}
}

func TestGetRating_NotFound(t *testing.T) {
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo)
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
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo)
	ctx := context.Background()

	if err := svc.Rate(ctx, "media-1", "profile-1", 4); err != nil {
		t.Fatal(err)
	}
	rt, err := svc.GetRating(ctx, "media-1", "profile-1")
	if err != nil || rt == nil || rt.Value != 4 {
		t.Fatalf("got %+v, err %v", rt, err)
	}
}

func TestDeleteRating(t *testing.T) {
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo)
	ctx := context.Background()

	svc.Rate(ctx, "media-1", "profile-1", 3)
	svc.DeleteRating(ctx, "media-1", "profile-1")
	rt, err := svc.GetRating(ctx, "media-1", "profile-1")
	if err != nil || rt != nil {
		t.Fatalf("expected nil after delete, got %+v, err %v", rt, err)
	}
}

func TestGetAggregates(t *testing.T) {
	repo := fakes.NewFakeRepository()
	svc := apprating.NewService(repo)
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
