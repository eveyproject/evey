package user

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

var (
	// ErrScheduleNotKids: opening hours only apply to a kids profile.
	ErrScheduleNotKids = errors.New("opening hours apply to kids profiles only")
	// ErrNoParentPIN: no adult profile of the account has a PIN to unlock with.
	ErrNoParentPIN = errors.New("no parent PIN set on this account")
	// ErrWrongParentPIN: the PIN matches no adult profile of the account.
	ErrWrongParentPIN = errors.New("wrong parent PIN")
)

// Clock gives the server's time zone (config.ServerClock).
type Clock interface {
	Location() *time.Location
}

// profileSnapshotTTL bounds how long a profile read is reused by the per-request
// check. The open/closed answer is recomputed from it on every call, so the
// profile still closes on the minute; only a schedule change made elsewhere
// waits this long (changes made here invalidate at once).
const profileSnapshotTTL = 15 * time.Second

// ProfileSchedules answers "may this profile be used now" for every request,
// and changes a kids profile's opening hours.
type ProfileSchedules struct {
	profiles user.ProfileRepository
	hasher   user.PasswordHasher
	clock    Clock
	now      func() time.Time
	// pins is the wrong-PIN lockout shared with the parental routes: unlock
	// guesses count on the adult profiles whose PIN they test.
	pins *PINAttempts

	mu    sync.Mutex
	cache map[shared.ProfileID]cachedProfile
}

type cachedProfile struct {
	p  *user.Profile
	at time.Time
}

func NewProfileSchedules(profiles user.ProfileRepository, hasher user.PasswordHasher, clock Clock) *ProfileSchedules {
	return &ProfileSchedules{profiles: profiles, hasher: hasher, clock: clock, now: time.Now, pins: SharedPINAttempts(), cache: map[shared.ProfileID]cachedProfile{}}
}

// WithPINAttempts swaps the wrong-PIN lockout (tests).
func (s *ProfileSchedules) WithPINAttempts(a *PINAttempts) *ProfileSchedules {
	s.pins = a
	return s
}

// WithNow pins the clock (tests).
func (s *ProfileSchedules) WithNow(now func() time.Time) *ProfileSchedules {
	s.now = now
	return s
}

func (s *ProfileSchedules) Location() *time.Location { return s.clock.Location() }

// Access is the per-request check. No profile (service tokens, API keys) is
// always open. A profile that cannot be read is an error: callers refuse.
func (s *ProfileSchedules) Access(ctx context.Context, profileID string) (user.ProfileAccess, error) {
	if profileID == "" {
		return user.ProfileAccess{Open: true}, nil
	}
	p, err := s.profile(ctx, shared.ProfileID(profileID))
	if err != nil {
		return user.ProfileAccess{}, err
	}
	return p.AccessAt(s.now(), s.clock.Location()), nil
}

func (s *ProfileSchedules) profile(ctx context.Context, id shared.ProfileID) (*user.Profile, error) {
	now := s.now()
	s.mu.Lock()
	if c, ok := s.cache[id]; ok && now.Sub(c.at) < profileSnapshotTTL {
		s.mu.Unlock()
		return c.p, nil
	}
	s.mu.Unlock()
	p, err := s.profiles.GetByID(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("profile schedule: %w", err)
	}
	s.mu.Lock()
	if len(s.cache) > 10_000 {
		clear(s.cache)
	}
	s.cache[id] = cachedProfile{p: p, at: now}
	s.mu.Unlock()
	return p, nil
}

// Invalidate drops a cached profile so the next request reads it again.
func (s *ProfileSchedules) Invalidate(id shared.ProfileID) {
	s.mu.Lock()
	delete(s.cache, id)
	s.mu.Unlock()
}

// Schedule returns the stored opening hours (nil when none).
func (s *ProfileSchedules) Schedule(ctx context.Context, id shared.ProfileID) (*user.Profile, user.ProfileAccess, error) {
	p, err := s.profiles.GetByID(ctx, id)
	if err != nil {
		return nil, user.ProfileAccess{}, err
	}
	return p, p.AccessAt(s.now(), s.clock.Location()), nil
}

// SetSchedule stores a kids profile's opening hours (nil removes them). A
// parent's PIN override is dropped: new hours apply from now.
func (s *ProfileSchedules) SetSchedule(ctx context.Context, p *user.Profile, sched *user.AccessSchedule) error {
	if !p.IsKids {
		return ErrScheduleNotKids
	}
	if sched != nil {
		if err := sched.Validate(); err != nil {
			return err
		}
	}
	if err := s.profiles.UpdateSchedule(ctx, p.ID, sched); err != nil {
		return err
	}
	if err := s.profiles.SetScheduleOverride(ctx, p.ID, nil); err != nil {
		return err
	}
	s.Invalidate(p.ID)
	return nil
}

// Unlock opens a closed kids profile until its next opening, with the PIN of
// an ADULT profile of the same account - the child's own PIN, if it has one,
// does not count. Returns when the override ends; a profile already open is
// left as it is (zero time) without comparing the PIN or touching any
// counter. A wrong PIN counts as a failure on every adult profile it was
// tested against, the same counters their own PIN dialogs use; ErrPINLocked
// while any of them sits out a lockout.
func (s *ProfileSchedules) Unlock(ctx context.Context, p *user.Profile, pin string) (time.Time, error) {
	now := s.now()
	loc := s.clock.Location()
	if acc := p.AccessAt(now, loc); acc.Open {
		return time.Time{}, nil
	}
	siblings, err := s.profiles.GetByUserID(ctx, p.UserID)
	if err != nil {
		return time.Time{}, err
	}
	var adults []*user.Profile
	var keys []string
	for _, sib := range siblings {
		if sib.IsKids || !sib.HasPIN() {
			continue
		}
		adults = append(adults, sib)
		keys = append(keys, string(sib.ID))
	}
	if len(adults) == 0 {
		return time.Time{}, ErrNoParentPIN
	}
	hit, err := s.pins.Attempt(ctx, keys, pin, func() (string, error) {
		for _, sib := range adults {
			ok, err := s.hasher.Verify(pin, sib.Parental.PINHash)
			if err != nil {
				return "", err
			}
			if ok {
				return string(sib.ID), nil
			}
		}
		return "", nil
	})
	if err != nil {
		return time.Time{}, err
	}
	if hit == "" {
		return time.Time{}, ErrWrongParentPIN
	}
	until := p.OverrideUntil(now, loc)
	if err := s.profiles.SetScheduleOverride(ctx, p.ID, &until); err != nil {
		return time.Time{}, err
	}
	s.Invalidate(p.ID)
	return until, nil
}
