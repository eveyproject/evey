package fakes_test

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user/fakes"
)

func TestLibraryAccess_GrantAndGetByUserID(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")
	repo.Grant(ctx, "u1", "lib-b")

	ids, err := repo.GetByUserID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 2 {
		t.Fatalf("expected 2 library IDs, got %d", len(ids))
	}
}

func TestLibraryAccess_NoRestrictions_FullAccess(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	// No grants = full access
	ids, err := repo.GetByUserID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if ids != nil {
		t.Fatalf("expected nil (no restrictions), got %v", ids)
	}

	ok, err := repo.HasAccess(ctx, "u1", "any-lib")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected full access when no restrictions")
	}
}

func TestLibraryAccess_HasAccess_Restricted(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")

	ok, err := repo.HasAccess(ctx, "u1", "lib-a")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected access to granted library")
	}

	ok, err = repo.HasAccess(ctx, "u1", "lib-b")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected no access to non-granted library")
	}
}

func TestLibraryAccess_Revoke(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")
	repo.Grant(ctx, "u1", "lib-b")
	repo.Revoke(ctx, "u1", "lib-a")

	ids, err := repo.GetByUserID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 || ids[0] != "lib-b" {
		t.Fatalf("expected [lib-b], got %v", ids)
	}
}

func TestLibraryAccess_RevokeAll_RestoresFullAccess(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")
	repo.Revoke(ctx, "u1", "lib-a")

	// After revoking all, user should have full access again
	ok, err := repo.HasAccess(ctx, "u1", "any-lib")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Fatal("expected full access after revoking all restrictions")
	}
}

func TestLibraryAccess_ListUsersWithRestrictions(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")
	repo.Grant(ctx, "u2", "lib-b")
	repo.Grant(ctx, "u2", "lib-c")

	result, err := repo.ListUsersWithRestrictions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 users with restrictions, got %d", len(result))
	}
	if len(result["u1"]) != 1 {
		t.Fatalf("expected 1 library for u1, got %d", len(result["u1"]))
	}
	if len(result["u2"]) != 2 {
		t.Fatalf("expected 2 libraries for u2, got %d", len(result["u2"]))
	}
}

func TestLibraryAccess_GrantIdempotent(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")
	repo.Grant(ctx, "u1", "lib-a") // duplicate

	ids, err := repo.GetByUserID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 1 {
		t.Fatalf("expected 1 library after duplicate grant, got %d", len(ids))
	}
}

// GrantMany inserts all library IDs in one call; empty slice is a no-op.
func TestLibraryAccess_GrantMany(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	if err := repo.GrantMany(ctx, "u1", nil); err != nil {
		t.Fatalf("empty GrantMany should be no-op, got %v", err)
	}
	ids, _ := repo.GetByUserID(ctx, "u1")
	if len(ids) != 0 {
		t.Fatalf("empty GrantMany created %d grants", len(ids))
	}

	if err := repo.GrantMany(ctx, "u1", []shared.LibraryID{"lib-a", "lib-b", "lib-c"}); err != nil {
		t.Fatalf("GrantMany: %v", err)
	}
	ids, _ = repo.GetByUserID(ctx, "u1")
	if len(ids) != 3 {
		t.Fatalf("expected 3 grants, got %d", len(ids))
	}

	// Duplicate: library already granted → idempotent.
	if err := repo.GrantMany(ctx, "u1", []shared.LibraryID{"lib-a", "lib-d"}); err != nil {
		t.Fatalf("GrantMany (dup): %v", err)
	}
	ids, _ = repo.GetByUserID(ctx, "u1")
	if len(ids) != 4 {
		t.Fatalf("expected 4 unique grants after dup GrantMany, got %d", len(ids))
	}
}

func TestLibraryAccess_DeleteByUserID(t *testing.T) {
	repo := fakes.NewFakeLibraryAccessRepository()
	ctx := context.Background()

	repo.Grant(ctx, "u1", "lib-a")
	repo.Grant(ctx, "u1", "lib-b")
	repo.DeleteByUserID(ctx, "u1")

	ids, err := repo.GetByUserID(ctx, "u1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected 0 libraries after delete, got %d", len(ids))
	}
}
