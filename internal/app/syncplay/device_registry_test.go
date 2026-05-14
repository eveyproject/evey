package syncplay

import (
	"encoding/json"
	"errors"
	"sync"
	"testing"

	"github.com/evey-project/evey/internal/domain/syncplay"
	"github.com/evey-project/evey/internal/domain/syncplay/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceRegistry_RegisterAndList(t *testing.T) {
	r := NewDeviceRegistry()
	s1 := fakes.NewFakeMessageSender()
	s2 := fakes.NewFakeMessageSender()

	r.Register("user-1", "dev-tv", "Living room TV", syncplay.DeviceTypeTV, s1)
	r.Register("user-1", "dev-phone", "Pixel", syncplay.DeviceTypeMobile, s2)
	// A different user's device must not leak into user-1's list.
	r.Register("user-2", "dev-other", "Someone else", syncplay.DeviceTypeWeb, fakes.NewFakeMessageSender())

	devs := r.List("user-1")
	require.Len(t, devs, 2)
	// Sorted by name: "Living room TV" < "Pixel".
	assert.Equal(t, "Living room TV", devs[0].Name)
	assert.Equal(t, "dev-tv", devs[0].ID)
	assert.Equal(t, syncplay.DeviceTypeTV, devs[0].Type)
	assert.Equal(t, "Pixel", devs[1].Name)

	assert.Len(t, r.List("user-2"), 1)
	assert.Empty(t, r.List("user-unknown"))
}

func TestDeviceRegistry_ReconnectReplaces(t *testing.T) {
	r := NewDeviceRegistry()
	old := fakes.NewFakeMessageSender()
	fresh := fakes.NewFakeMessageSender()

	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, old)
	// Same deviceID reconnecting (app restarted) must supersede the stale
	// connection, not create a duplicate.
	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, fresh)

	require.Len(t, r.List("user-1"), 1)

	err := r.SendCommand("user-1", "dev-tv", syncplay.DeviceCommand{Type: syncplay.DeviceCmdPause})
	require.NoError(t, err)
	assert.Empty(t, old.Received(), "stale connection must not receive commands")
	assert.Len(t, fresh.Received(), 1, "fresh connection receives the command")
}

func TestDeviceRegistry_Unregister(t *testing.T) {
	r := NewDeviceRegistry()
	s := fakes.NewFakeMessageSender()
	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, s)

	r.Unregister("user-1", "dev-tv", s)
	assert.Empty(t, r.List("user-1"))
	// Idempotent — unknown device / already-removed is a no-op.
	r.Unregister("user-1", "dev-tv", s)
	r.Unregister("user-unknown", "whatever", nil)
}

// A stale disconnect must not evict a newer connection for the same
// deviceID. Unregister only removes the entry when the sender matches.
func TestDeviceRegistry_UnregisterStaleSenderNoOp(t *testing.T) {
	r := NewDeviceRegistry()
	stale := fakes.NewFakeMessageSender()
	fresh := fakes.NewFakeMessageSender()

	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, stale)
	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, fresh) // reconnect

	// The stale connection's defer fires late — it must not remove the
	// fresh entry.
	r.Unregister("user-1", "dev-tv", stale)
	require.Len(t, r.List("user-1"), 1, "fresh connection survives a stale disconnect")

	r.Unregister("user-1", "dev-tv", fresh)
	assert.Empty(t, r.List("user-1"))
}

func TestDeviceRegistry_SendCommand(t *testing.T) {
	r := NewDeviceRegistry()
	s := fakes.NewFakeMessageSender()
	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, s)

	cmd := syncplay.DeviceCommand{
		Type:       syncplay.DeviceCmdLoadAndPlay,
		MediaID:    "media-42",
		PositionMs: 90000,
	}
	require.NoError(t, r.SendCommand("user-1", "dev-tv", cmd))

	got := s.Received()
	require.Len(t, got, 1)
	var decoded syncplay.DeviceCommand
	require.NoError(t, json.Unmarshal(got[0], &decoded))
	assert.Equal(t, cmd, decoded)
}

func TestDeviceRegistry_SendCommand_NotConnected(t *testing.T) {
	r := NewDeviceRegistry()
	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())

	// Wrong device, wrong user, and a user with no devices at all.
	assert.ErrorIs(t, r.SendCommand("user-1", "dev-missing", syncplay.DeviceCommand{Type: "play"}), ErrDeviceNotConnected)
	assert.ErrorIs(t, r.SendCommand("user-2", "dev-tv", syncplay.DeviceCommand{Type: "play"}), ErrDeviceNotConnected)
}

func TestDeviceRegistry_SendCommand_SenderError(t *testing.T) {
	r := NewDeviceRegistry()
	s := fakes.NewFakeMessageSender()
	s.Err = errors.New("socket closed")
	r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, s)

	err := r.SendCommand("user-1", "dev-tv", syncplay.DeviceCommand{Type: "pause"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDeviceNotConnected, "a send failure is distinct from not-connected")
}

// The registry is hit concurrently by many WebSocket goroutines (each
// client connection) plus HTTP handlers listing/commanding. Race-detector
// guard.
func TestDeviceRegistry_ConcurrentSafe(t *testing.T) {
	r := NewDeviceRegistry()
	const workers = 16
	const iters = 200
	var wg sync.WaitGroup
	wg.Add(workers)
	for w := 0; w < workers; w++ {
		go func(w int) {
			defer wg.Done()
			uid := "user-1"
			did := "dev-" + string(rune('A'+w))
			for i := 0; i < iters; i++ {
				s := fakes.NewFakeMessageSender()
				r.Register(uid, did, "D", syncplay.DeviceTypeTV, s)
				_ = r.SendCommand(uid, did, syncplay.DeviceCommand{Type: "play"})
				_ = r.List(uid)
				r.Unregister(uid, did, s)
			}
		}(w)
	}
	wg.Wait()
}
