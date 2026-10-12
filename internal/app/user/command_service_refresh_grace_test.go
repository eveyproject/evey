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

// Multi-tab refresh race: two tabs share the same HttpOnly refresh cookie and
// fire /auth/refresh concurrently. The winner rotates; the loser used to be
// rejected outright, killing that tab's whole session (every API call 401s
// until a manual reload - observed live on /requests, breaking the page).
// With the rotation grace window the loser exchanges the just-rotated token
// for its OWN fresh session instead.
func TestRefreshToken_MultiTabRaceLoser_WithinGrace_GetsFreshSession(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M", IsDefault: true})

	raw := "shared-cookie-refresh-tok"
	h := sha256.Sum256([]byte(raw))
	if err := d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt1", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	// Tab A wins the rotation.
	_, refreshA, err := d.svc.RefreshToken(ctx, raw, "", "")
	if err != nil {
		t.Fatalf("winner refresh failed: %v", err)
	}

	// Tab B replays the same (now rotated) token milliseconds later.
	accessB, refreshB, err := d.svc.RefreshToken(ctx, raw, "", "")
	if err != nil {
		t.Fatalf("race loser within the grace window must get a fresh session, got: %v", err)
	}
	if refreshB == "" || refreshB == refreshA {
		t.Fatalf("loser must receive its OWN refresh token (sibling session), got %q vs winner %q", refreshB, refreshA)
	}
	if claims, cerr := d.tokens.ValidateAccessToken(accessB); cerr != nil || claims.UserID != "u1" {
		t.Fatalf("loser's access token must be valid for the user: claims=%v err=%v", claims, cerr)
	}

	// Both sessions are independently usable afterwards.
	if _, _, err := d.svc.RefreshToken(ctx, refreshA, "", ""); err != nil {
		t.Fatalf("winner's rotated token must stay usable: %v", err)
	}
	if _, _, err := d.svc.RefreshToken(ctx, refreshB, "", ""); err != nil {
		t.Fatalf("loser's sibling token must stay usable: %v", err)
	}
}

// Outside the grace window a rotated token is a genuine replay (stolen token,
// stale client) and stays rejected - the grace never weakens late replays.
func TestRefreshToken_ReplayAfterGrace_Rejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M", IsDefault: true})

	raw := "long-rotated-refresh-tok"
	h := sha256.Sum256([]byte(raw))
	rotatedLongAgo := time.Now().Add(-2 * time.Minute)
	if err := d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-old", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		Used: true, UsedAt: &rotatedLongAgo,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := d.svc.RefreshToken(ctx, raw, "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a rotation older than the grace window must be rejected, got: %v", err)
	}
}

// A rotated row carrying no rotation timestamp (legacy row claimed before the
// used_at column existed) is treated as outside the grace - fail closed.
func TestRefreshToken_UsedWithoutTimestamp_Rejected(t *testing.T) {
	d := newAuthTestDeps()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p1", UserID: "u1", Name: "M", IsDefault: true})

	raw := "legacy-used-refresh-tok"
	h := sha256.Sum256([]byte(raw))
	if err := d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-legacy", UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		Used:      true,
		ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatal(err)
	}

	if _, _, err := d.svc.RefreshToken(ctx, raw, "", ""); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("a used row without used_at must be rejected, got: %v", err)
	}
}
