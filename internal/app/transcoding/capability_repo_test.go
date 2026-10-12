package transcoding

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	domainshared "github.com/eveyproject/evey/internal/domain/shared"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// fakeConfigStore is an in-memory ServerConfigRepository for tests.
type fakeConfigStore struct {
	mu      sync.Mutex
	entries map[string]string
	getErr  error
}

func newFakeConfigStore() *fakeConfigStore {
	return &fakeConfigStore{entries: make(map[string]string)}
}

func (s *fakeConfigStore) Get(_ context.Context, key string) (*domainshared.ServerConfigEntry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.getErr != nil {
		return nil, s.getErr
	}
	v, ok := s.entries[key]
	if !ok {
		return nil, nil
	}
	return &domainshared.ServerConfigEntry{Key: key, Value: v, UpdatedAt: time.Now()}, nil
}

func (s *fakeConfigStore) Set(_ context.Context, key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries[key] = value
	return nil
}

func (s *fakeConfigStore) Delete(_ context.Context, key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, key)
	return nil
}

func (s *fakeConfigStore) List(_ context.Context) ([]*domainshared.ServerConfigEntry, error) {
	return nil, nil
}

func TestCapabilityRepo_GetEmpty_ReturnsNotFound(t *testing.T) {
	repo := NewCapabilityRepoAdapter(newFakeConfigStore())
	cap, found, err := repo.Get(context.Background())
	require.NoError(t, err)
	assert.False(t, found, "empty store must report no calibration")
	assert.Zero(t, cap.SoftwareMaxHeight)
}

func TestCapabilityRepo_SetThenGet_Roundtrips(t *testing.T) {
	store := newFakeConfigStore()
	repo := NewCapabilityRepoAdapter(store)
	in := domaintranscoding.EncodeCapability{
		SoftwareMaxHeight: 1080,
		HWMaxHeight:       2160,
		HWBackend:         "nvenc",
		CPUSignature:      "deadbeef",
		BenchmarkedAt:     time.Now().UTC().Truncate(time.Second),
	}
	require.NoError(t, repo.Set(context.Background(), in))

	out, found, err := repo.Get(context.Background())
	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, in.SoftwareMaxHeight, out.SoftwareMaxHeight)
	assert.Equal(t, in.HWMaxHeight, out.HWMaxHeight)
	assert.Equal(t, in.HWBackend, out.HWBackend)
	assert.Equal(t, in.CPUSignature, out.CPUSignature)
	assert.WithinDuration(t, in.BenchmarkedAt, out.BenchmarkedAt, time.Second)
}

func TestCapabilityRepo_RefuseZeroSoftwareMaxHeight(t *testing.T) {
	repo := NewCapabilityRepoAdapter(newFakeConfigStore())
	err := repo.Set(context.Background(), domaintranscoding.EncodeCapability{})
	require.Error(t, err, "must refuse to persist a useless zero calibration")
}

func TestCapabilityRepo_CorruptJSON_TreatedAsNotFound(t *testing.T) {
	store := newFakeConfigStore()
	// Bypass Set's validation by writing garbage directly.
	require.NoError(t, store.Set(context.Background(), CapabilityConfigKey, "{not valid json"))

	repo := NewCapabilityRepoAdapter(store)
	cap, found, err := repo.Get(context.Background())
	// We want a graceful "no calibration" rather than a boot-blocking
	// error: the caller will re-run the benchmark and overwrite the bad
	// row. Crashing on corrupt config would make a single bad write
	// permanently brick the server.
	require.NoError(t, err)
	assert.False(t, found)
	assert.Zero(t, cap.SoftwareMaxHeight)
}

func TestCapabilityRepo_StoreError_PropagatesOnGet(t *testing.T) {
	store := newFakeConfigStore()
	store.getErr = errors.New("db down")
	repo := NewCapabilityRepoAdapter(store)
	_, _, err := repo.Get(context.Background())
	require.Error(t, err, "real DB errors must propagate (not be swallowed like corrupt JSON)")
}
