package fakes

import (
	"context"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
)

type FakeProfileRepository struct {
	mu    sync.RWMutex
	items map[string]*user.Profile
	Err   error
}

func NewFakeProfileRepository() *FakeProfileRepository {
	return &FakeProfileRepository{items: make(map[string]*user.Profile)}
}

func (r *FakeProfileRepository) Create(_ context.Context, p *user.Profile) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	r.items[string(p.ID)] = cloneProfile(p)
	return nil
}

// CreateWithinCap mirrors the repo: count + insert under the same lock so the
// cap holds atomically. maxProfiles <= 0 means unlimited.
func (r *FakeProfileRepository) CreateWithinCap(_ context.Context, p *user.Profile, maxProfiles int) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if maxProfiles > 0 {
		n := 0
		for _, existing := range r.items {
			if existing.UserID == p.UserID {
				n++
			}
		}
		if n >= maxProfiles {
			return user.ErrProfileCapReached
		}
	}
	r.items[string(p.ID)] = cloneProfile(p)
	return nil
}

func (r *FakeProfileRepository) GetByID(_ context.Context, id shared.ProfileID) (*user.Profile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	p, ok := r.items[string(id)]
	if !ok {
		return nil, ErrNotFound
	}
	return cloneProfile(p), nil
}

func (r *FakeProfileRepository) GetByUserID(_ context.Context, userID shared.UserID) ([]*user.Profile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	var result []*user.Profile
	for _, p := range r.items {
		if p.UserID == userID {
			result = append(result, cloneProfile(p))
		}
	}
	return result, nil
}

func (r *FakeProfileRepository) Update(_ context.Context, p *user.Profile) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(p.ID)]; !ok {
		return ErrNotFound
	}
	r.items[string(p.ID)] = cloneProfile(p)
	return nil
}

// UpdateAvatarPath mirrors the postgres column-scoped write: it mutates ONLY
// AvatarPath on the stored record, leaving Parental (and every other field)
// untouched - a stale caller can never resurrect an old PIN through this path.
func (r *FakeProfileRepository) UpdateAvatarPath(_ context.Context, id shared.ProfileID, avatarPath *string) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	p, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	p.AvatarPath = avatarPath
	return nil
}

// UpdateParentalSettings mirrors the postgres column-scoped write: it mutates
// ONLY Parental on the stored record, leaving AvatarPath (and every other
// field) untouched.
func (r *FakeProfileRepository) UpdateParentalSettings(_ context.Context, id shared.ProfileID, settings *user.ParentalSettings) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	p, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	if settings == nil {
		p.Parental = nil
		return nil
	}
	cp := *settings
	p.Parental = &cp
	return nil
}

// TouchLastActive mirrors the postgres column-scoped write: only LastActiveAt
// moves on the stored record.
func (r *FakeProfileRepository) TouchLastActive(_ context.Context, id shared.ProfileID, at time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	t := at
	p.LastActiveAt = &t
	return nil
}

func (r *FakeProfileRepository) GetAll(_ context.Context) ([]*user.Profile, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	out := make([]*user.Profile, 0, len(r.items))
	for _, p := range r.items {
		out = append(out, p)
	}
	return out, nil
}

func (r *FakeProfileRepository) SetDefault(_ context.Context, userID shared.UserID, profileID shared.ProfileID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	target, ok := r.items[string(profileID)]
	if !ok || target.UserID != userID {
		return ErrNotFound
	}
	for _, p := range r.items {
		if p.UserID == userID {
			p.IsDefault = false
		}
	}
	target.IsDefault = true
	return nil
}

func (r *FakeProfileRepository) Delete(_ context.Context, id shared.ProfileID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	if _, ok := r.items[string(id)]; !ok {
		return ErrNotFound
	}
	delete(r.items, string(id))
	return nil
}

// cloneProfile detaches the stored record from the caller's pointer, so a
// service that mutates a profile in memory without persisting it is
// distinguishable from one that does. See cloneUser for the full rationale.
func cloneProfile(p *user.Profile) *user.Profile {
	if p == nil {
		return nil
	}
	cp := *p
	if p.Parental != nil {
		par := *p.Parental
		cp.Parental = &par
	}
	if p.LastActiveAt != nil {
		t := *p.LastActiveAt
		cp.LastActiveAt = &t
	}
	return &cp
}

// UpdateSchedule mirrors the postgres column-scoped write.
func (r *FakeProfileRepository) UpdateSchedule(_ context.Context, id shared.ProfileID, s *user.AccessSchedule) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	if s == nil {
		p.Schedule = nil
		return nil
	}
	cp := *s
	p.Schedule = &cp
	return nil
}

// SetScheduleOverride mirrors the postgres column-scoped write.
func (r *FakeProfileRepository) SetScheduleOverride(_ context.Context, id shared.ProfileID, until *time.Time) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.items[string(id)]
	if !ok {
		return ErrNotFound
	}
	if until == nil {
		p.ScheduleOverrideUntil = nil
		return nil
	}
	t := *until
	p.ScheduleOverrideUntil = &t
	return nil
}
