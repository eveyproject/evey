package fakes

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

func validToken(id, userID, hash string, expiresAt time.Time) *user.PasswordResetToken {
	return &user.PasswordResetToken{
		ID:        id,
		UserID:    shared.UserID(userID),
		TokenHash: hash,
		ExpiresAt: expiresAt,
		Used:      false,
		CreatedAt: time.Now(),
	}
}

func TestCreate(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	ctx := context.Background()
	tok := validToken("t1", "u1", "hash1", time.Now().Add(time.Hour))

	if err := repo.Create(ctx, tok); err != nil {
		t.Fatal(err)
	}
	if repo.Len() != 1 {
		t.Fatalf("expected 1 token, got %d", repo.Len())
	}
}

func TestCreate_Error(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	repo.Err = errors.New("db down")
	err := repo.Create(context.Background(), validToken("t1", "u1", "h", time.Now()))
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetByTokenHash_Found(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	ctx := context.Background()
	tok := validToken("t1", "u1", "secret-hash", time.Now().Add(time.Hour))
	_ = repo.Create(ctx, tok)

	got, err := repo.GetByTokenHash(ctx, "secret-hash")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "t1" {
		t.Fatalf("expected t1, got %s", got.ID)
	}
}

func TestGetByTokenHash_NotFound(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	_, err := repo.GetByTokenHash(context.Background(), "nonexistent")
	if !errors.Is(err, shared.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestMarkUsed(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	ctx := context.Background()
	tok := validToken("t1", "u1", "h1", time.Now().Add(time.Hour))
	_ = repo.Create(ctx, tok)

	if err := repo.MarkUsed(ctx, "t1"); err != nil {
		t.Fatal(err)
	}
	got := repo.Get("t1")
	if got == nil || !got.Used {
		t.Fatal("expected token to be marked used")
	}
}

func TestDeleteByUserID(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	ctx := context.Background()
	_ = repo.Create(ctx, validToken("t1", "u1", "h1", time.Now().Add(time.Hour)))
	_ = repo.Create(ctx, validToken("t2", "u1", "h2", time.Now().Add(time.Hour)))
	_ = repo.Create(ctx, validToken("t3", "u2", "h3", time.Now().Add(time.Hour)))

	if err := repo.DeleteByUserID(ctx, "u1"); err != nil {
		t.Fatal(err)
	}
	if repo.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", repo.Len())
	}
	if repo.Get("t3") == nil {
		t.Fatal("wrong token deleted")
	}
}

func TestDeleteExpired_RemovesExpiredAndUsed(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	ctx := context.Background()

	expired := validToken("t-expired", "u1", "h1", time.Now().Add(-time.Hour))
	_ = repo.Create(ctx, expired)

	used := validToken("t-used", "u1", "h2", time.Now().Add(time.Hour))
	used.Used = true
	_ = repo.Create(ctx, used)

	valid := validToken("t-valid", "u1", "h3", time.Now().Add(time.Hour))
	_ = repo.Create(ctx, valid)

	if err := repo.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if repo.Len() != 1 {
		t.Fatalf("expected 1 remaining, got %d", repo.Len())
	}
	if repo.Get("t-valid") == nil {
		t.Fatal("valid token should remain")
	}
}

func TestDeleteExpired_KeepsAllValid(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	ctx := context.Background()
	_ = repo.Create(ctx, validToken("t1", "u1", "h1", time.Now().Add(time.Hour)))
	_ = repo.Create(ctx, validToken("t2", "u2", "h2", time.Now().Add(2*time.Hour)))

	if err := repo.DeleteExpired(ctx); err != nil {
		t.Fatal(err)
	}
	if repo.Len() != 2 {
		t.Fatalf("expected 2, got %d", repo.Len())
	}
}

func TestAllMethods_ReturnErrWhenSet(t *testing.T) {
	repo := NewFakePasswordResetTokenRepository()
	repo.Err = errors.New("forced")
	ctx := context.Background()
	tok := validToken("t1", "u1", "h1", time.Now())

	if err := repo.Create(ctx, tok); err == nil {
		t.Fatal("Create: expected error")
	}
	if _, err := repo.GetByTokenHash(ctx, "h1"); err == nil {
		t.Fatal("GetByTokenHash: expected error")
	}
	if err := repo.MarkUsed(ctx, "t1"); err == nil {
		t.Fatal("MarkUsed: expected error")
	}
	if err := repo.DeleteByUserID(ctx, "u1"); err == nil {
		t.Fatal("DeleteByUserID: expected error")
	}
	if err := repo.DeleteExpired(ctx); err == nil {
		t.Fatal("DeleteExpired: expected error")
	}
}
