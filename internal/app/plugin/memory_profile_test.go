package plugin

import (
	"context"
	"testing"

	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
)

type fakeConfigRepo struct {
	values map[string]string
}

func (r *fakeConfigRepo) Get(_ context.Context, key string) (*shared.ServerConfigEntry, error) {
	if v, ok := r.values[key]; ok {
		return &shared.ServerConfigEntry{Key: key, Value: v}, nil
	}
	return nil, nil
}

func (r *fakeConfigRepo) Set(_ context.Context, key, value string) error {
	r.values[key] = value
	return nil
}

func (r *fakeConfigRepo) Delete(_ context.Context, key string) error {
	delete(r.values, key)
	return nil
}

func (r *fakeConfigRepo) List(_ context.Context) ([]*shared.ServerConfigEntry, error) {
	return nil, nil
}

func withBudget(t *testing.T, bytes int64, ok bool) {
	t.Helper()
	orig := memoryBudget
	memoryBudget = func() (int64, bool) { return bytes, ok }
	t.Cleanup(func() { memoryBudget = orig })
}

func TestResolveMemoryProfile_AutoLowOnSmallBudget(t *testing.T) {
	withBudget(t, 2<<30, true) // 2 GiB NAS
	profile, budget := ResolveMemoryProfile(context.Background(), nil)
	assert.Equal(t, MemoryProfileLow, profile)
	assert.Equal(t, int64(2<<30), budget)
}

func TestResolveMemoryProfile_AutoNormalOnBigBudget(t *testing.T) {
	withBudget(t, 16<<30, true)
	profile, _ := ResolveMemoryProfile(context.Background(), nil)
	assert.Equal(t, MemoryProfileNormal, profile)
}

func TestResolveMemoryProfile_StoredValueWins(t *testing.T) {
	withBudget(t, 16<<30, true) // big box, but the operator wants low
	repo := &fakeConfigRepo{values: map[string]string{KeyMemoryProfile: MemoryProfileLow}}
	profile, _ := ResolveMemoryProfile(context.Background(), repo)
	assert.Equal(t, MemoryProfileLow, profile)
}

func TestResolveMemoryProfile_InvalidStoredFallsBackToAuto(t *testing.T) {
	withBudget(t, 2<<30, true)
	repo := &fakeConfigRepo{values: map[string]string{KeyMemoryProfile: "turbo"}}
	profile, _ := ResolveMemoryProfile(context.Background(), repo)
	assert.Equal(t, MemoryProfileLow, profile)
}

func TestResolveMemoryProfile_UnknownBudgetIsNormal(t *testing.T) {
	withBudget(t, 0, false)
	profile, budget := ResolveMemoryProfile(context.Background(), nil)
	assert.Equal(t, MemoryProfileNormal, profile)
	assert.Equal(t, int64(0), budget)
}
