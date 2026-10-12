package grpcapi

import (
	"context"
	"strconv"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

// ProfileHoursReader says whether a profile may be used now and, if so, when
// its opening ends (app/user.ProfileSchedules).
type ProfileHoursReader interface {
	Access(ctx context.Context, profileID string) (user.ProfileAccess, error)
	Location() *time.Location
}

// closingWarnLead is how long before a kids profile closes its player is
// told (user, 2026-10-01: a warning, then the stop).
const closingWarnLead = 10 * time.Minute

// closingWarnings remembers which session was already warned for which
// closing, so the warning is a toast, not one per progress tick.
type closingWarnings struct {
	mu   sync.Mutex
	seen map[string]time.Time
}

func (c *closingWarnings) first(sessionID string, closesAt time.Time) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.seen == nil {
		c.seen = map[string]time.Time{}
	}
	if prev, ok := c.seen[sessionID]; ok && prev.Equal(closesAt) {
		return false
	}
	if len(c.seen) > 5000 {
		clear(c.seen)
	}
	c.seen[sessionID] = closesAt
	return true
}

// closingSoonMessage returns the "message:" remote command warning a kids
// profile's player that its opening ends within closingWarnLead, once per
// session and closing. Empty when there is nothing to say.
func closingSoonMessage(ctx context.Context, hours ProfileHoursReader, warned *closingWarnings, translator *eveyi18n.Translator, profileID, sessionID string, now time.Time) string {
	if hours == nil || profileID == "" {
		return ""
	}
	acc, err := hours.Access(ctx, profileID)
	if err != nil || !acc.Open || acc.ClosesAt.IsZero() {
		return ""
	}
	left := acc.ClosesAt.Sub(now)
	if left <= 0 || left > closingWarnLead || !warned.first(sessionID, acc.ClosesAt) {
		return ""
	}
	params := map[string]string{
		"minutes": strconv.Itoa(int((left + time.Minute - 1) / time.Minute)),
		"time":    acc.ClosesAt.In(hours.Location()).Format("15:04"),
	}
	msg := "auth.profile.closingSoon"
	if translator != nil {
		msg = translator.T(eveyi18n.LocaleFromCtx(ctx), msg, params)
	}
	return "message:" + msg
}
