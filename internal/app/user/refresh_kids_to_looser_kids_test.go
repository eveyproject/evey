package user

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

func seedKidsSwitch(t *testing.T, d *authTestDeps, from string) string {
	t.Helper()
	ctx := context.Background()
	seedAuthUser(t, d, "u1", "alice", "pw", false, true)
	young, older := 7, 16
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-adult", UserID: "u1", Name: "Adult", IsDefault: true, Parental: &user.ParentalSettings{PINHash: "h"}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-young", UserID: "u1", Name: "Young", IsKids: true, Parental: &user.ParentalSettings{MaxAge: &young}})
	_ = d.profiles.Create(ctx, &user.Profile{ID: "p-older", UserID: "u1", Name: "Older", IsKids: true, Parental: &user.ParentalSettings{MaxAge: &older}})
	raw := "tok-" + from
	h := sha256.Sum256([]byte(raw))
	pid := shared.ProfileID(from)
	_ = d.refreshTkns.Create(ctx, &user.RefreshToken{
		ID: "rt-" + from, UserID: "u1", TokenHash: hex.EncodeToString(h[:]),
		ProfileID: &pid, ExpiresAt: time.Now().Add(24 * time.Hour),
	})
	return raw
}

// A kids profile switching to a LESS restrictive kids profile (higher age cap)
// needs a guardian's PIN; the other way round stays free.
func TestRefreshToken_KidsToLooserKids_RequiresGuardianPIN(t *testing.T) {
	ctx := context.Background()

	d := newAuthTestDeps()
	raw := seedKidsSwitch(t, d, "p-young")
	_, _, err := d.svc.RefreshToken(ctx, raw, "p-older", "")
	require.ErrorIs(t, err, ErrPINRequired)

	proof, err := d.tokens.GeneratePINToken("p-adult", time.Minute)
	require.NoError(t, err)
	_, _, err = d.svc.RefreshToken(ctx, raw, "p-older", proof)
	require.NoError(t, err)

	d2 := newAuthTestDeps()
	raw2 := seedKidsSwitch(t, d2, "p-older")
	_, _, err = d2.svc.RefreshToken(ctx, raw2, "p-young", "")
	require.NoError(t, err, "a stricter profile needs no PIN")
}
