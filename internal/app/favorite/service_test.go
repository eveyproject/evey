package favorite

import (
	"context"
	"testing"

	"github.com/gravity-zero/evey/internal/domain/favorite/fakes"
)

func newTestService() *Service {
	return NewService(fakes.NewFakeRepository())
}

func TestToggle_AddAndRemove(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	added, err := svc.Toggle(ctx, "p1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if !added {
		t.Fatal("expected added=true")
	}

	ok, _ := svc.IsFavorite(ctx, "p1", "m1")
	if !ok {
		t.Fatal("expected favorite")
	}

	removed, err := svc.Toggle(ctx, "p1", "m1")
	if err != nil {
		t.Fatal(err)
	}
	if removed {
		t.Fatal("expected added=false after toggle off")
	}

	ok, _ = svc.IsFavorite(ctx, "p1", "m1")
	if ok {
		t.Fatal("expected not favorite")
	}
}

func TestList(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	svc.Toggle(ctx, "p1", "m1")
	svc.Toggle(ctx, "p1", "m2")
	svc.Toggle(ctx, "p1", "m3")
	svc.Toggle(ctx, "p2", "m1")

	items, err := svc.List(ctx, "p1", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 3 {
		t.Fatalf("expected 3, got %d", len(items))
	}

	items, err = svc.List(ctx, "p2", 50, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 {
		t.Fatalf("expected 1, got %d", len(items))
	}
}

func TestList_DefaultLimit(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	items, err := svc.List(ctx, "p1", 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	if items != nil && len(items) != 0 {
		t.Fatal("expected empty")
	}
}

func TestIsFavorite_NotFound(t *testing.T) {
	svc := newTestService()
	ctx := context.Background()

	ok, err := svc.IsFavorite(ctx, "p1", "m999")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected false")
	}
}
