package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"testing"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
)

// seedRefreshToken stores one refresh token for alice and returns its raw
// value, so a test can present it the way a client would.
func seedRefreshToken(t *testing.T, d *authTestDeps, id, raw string) {
	t.Helper()
	h := sha256.Sum256([]byte(raw))
	if err := d.refreshTkns.Create(context.Background(), &user.RefreshToken{
		ID: id, UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}
}

// seedAuthedUser is the common premise of the tests below: one enabled user
// with one profile.
func seedAuthedUser(t *testing.T, d *authTestDeps) {
	t.Helper()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(context.Background(), &user.Profile{ID: "p1", UserID: "u1", Name: "M", IsDefault: true})
}

// The case that cost a living-room television its session (2026-09-19): the
// server rotated the token, the app was killed before it could write the new
// one, and every later refresh presented a token the server had consumed.
// Past the grace that used to be a flat refusal - a login screen with no
// keyboard. The successor was never used by anyone, which is the proof that
// the answer was lost rather than replayed.
func TestRefreshToken_InterruptedRotation_ReissuesInsteadOfKillingTheSession(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthedUser(t, d)
	seedRefreshToken(t, d, "rt1", "lost-answer-token")

	// The rotation the client never heard about.
	_, newRefresh, err := d.svc.RefreshToken(ctx, "lost-answer-token", "", "")
	if err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}
	if newRefresh == "" {
		t.Fatal("premise: the rotation must have issued a successor")
	}
	rt, err := d.refreshTkns.GetByID(ctx, "rt1")
	if err != nil {
		t.Fatal(err)
	}
	if rt.SuccessorID == nil {
		t.Fatal("the rotation must record which row it was exchanged for")
	}
	successorID := *rt.SuccessorID

	// Hours later (well past the grace) the client comes back with the only
	// token it ever had.
	aged := time.Now().Add(-2 * time.Hour)
	rt.UsedAt = &aged
	d.refreshTkns.Replace(rt)

	access, reissued, err := d.svc.RefreshToken(ctx, "lost-answer-token", "", "")
	if err != nil {
		t.Fatalf("a client whose rotation answer was lost must get a session back, got: %v", err)
	}
	if reissued == "" || reissued == newRefresh {
		t.Fatalf("it must get a token of its own, not the lost one: %q vs %q", reissued, newRefresh)
	}
	if claims, cerr := d.tokens.ValidateAccessToken(access); cerr != nil || claims.UserID != "u1" {
		t.Fatalf("the access token must be valid for the user: claims=%v err=%v", claims, cerr)
	}
	// The orphan is burned: whoever may have intercepted that lost answer on
	// the wire cannot spend it now.
	if _, gErr := d.refreshTkns.GetByID(ctx, successorID); gErr == nil {
		t.Fatal("the never-delivered successor must be destroyed, not left usable")
	}
}

// The mirror case, and the reason the alibi is checked rather than assumed: a
// stolen token replayed long after its rotation. The legitimate client DID
// receive the successor and used it, so there is no lost answer to recover.
func TestRefreshToken_ReplayAfterTheSuccessorWasUsed_StillRefused(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthedUser(t, d)
	seedRefreshToken(t, d, "rt1", "stolen-token")

	_, legitimate, err := d.svc.RefreshToken(ctx, "stolen-token", "", "")
	if err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}
	// The real client carries on normally, consuming its successor.
	if _, _, useErr := d.svc.RefreshToken(ctx, legitimate, "", ""); useErr != nil {
		t.Fatalf("the legitimate client must keep working: %v", useErr)
	}

	rt, err := d.refreshTkns.GetByID(ctx, "rt1")
	if err != nil {
		t.Fatal(err)
	}
	aged := time.Now().Add(-2 * time.Hour)
	rt.UsedAt = &aged
	d.refreshTkns.Replace(rt)

	if _, _, err := d.svc.RefreshToken(ctx, "stolen-token", "", ""); err == nil {
		t.Fatal("a replay whose successor was used must still be refused")
	}
}

// Rows rotated before the column existed carry no breadcrumb at all. No
// alibi, no exception: they stay refused.
func TestRefreshToken_RotatedWithoutSuccessor_StaysRefused(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthedUser(t, d)

	h := sha256.Sum256([]byte("legacy-token"))
	aged := time.Now().Add(-2 * time.Hour)
	if err := d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-legacy", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		Used: true, UsedAt: &aged, ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := d.svc.RefreshToken(ctx, "legacy-token", "", ""); err == nil {
		t.Fatal("a rotated token with no recorded successor must stay refused")
	}
}

// The alibi has a shelf life. A successor minted a day ago and never used is
// no longer a lost answer in flight: it is a token the client abandoned, and
// the old token presented with it is treated like any replay.
func TestRefreshToken_InterruptedRotation_TooOldToRecover(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthedUser(t, d)
	seedRefreshToken(t, d, "rt1", "abandoned-token")

	if _, _, err := d.svc.RefreshToken(ctx, "abandoned-token", "", ""); err != nil {
		t.Fatalf("first refresh failed: %v", err)
	}
	rt, err := d.refreshTkns.GetByID(ctx, "rt1")
	if err != nil || rt.SuccessorID == nil {
		t.Fatalf("premise: the rotation must record its successor (err=%v)", err)
	}
	successor, err := d.refreshTkns.GetByID(ctx, *rt.SuccessorID)
	if err != nil {
		t.Fatal(err)
	}
	long := time.Now().Add(-(rotationRecoveryWindow + time.Hour))
	rt.UsedAt = &long
	d.refreshTkns.Replace(rt)
	successor.CreatedAt = long
	successor.ExpiresAt = time.Now().Add(time.Hour)
	d.refreshTkns.Replace(successor)

	if _, _, err := d.svc.RefreshToken(ctx, "abandoned-token", "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a rotation older than the recovery window must be refused, got: %v", err)
	}
	// Refused, and the stale successor is not left live either.
	if _, gErr := d.refreshTkns.GetByID(ctx, successor.ID); gErr == nil {
		t.Fatal("the abandoned successor must be burned along with the refusal")
	}
}
