package syncplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"

	"github.com/eveyproject/evey/internal/domain/syncplay"
	"github.com/eveyproject/evey/internal/domain/syncplay/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeviceRegistry_RegisterAndList(t *testing.T) {
	r := NewDeviceRegistry()
	s1 := fakes.NewFakeMessageSender()
	s2 := fakes.NewFakeMessageSender()

	_, _ = r.Register("user-1", "dev-tv", "Living room TV", syncplay.DeviceTypeTV, s1)
	_, _ = r.Register("user-1", "dev-phone", "Pixel", syncplay.DeviceTypeMobile, s2)
	// A different user's device must not leak into user-1's list.
	_, _ = r.Register("user-2", "dev-other", "Someone else", syncplay.DeviceTypeWeb, fakes.NewFakeMessageSender())

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

	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, old)
	// Same deviceID reconnecting (app restarted) must supersede the stale
	// connection, not create a duplicate.
	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, fresh)

	require.Len(t, r.List("user-1"), 1)

	err := r.SendCommand("user-1", "dev-tv", syncplay.DeviceCommand{Type: syncplay.DeviceCmdPause})
	require.NoError(t, err)
	assert.Empty(t, old.Received(), "stale connection must not receive commands")
	assert.Len(t, fresh.Received(), 1, "fresh connection receives the command")
}

func TestDeviceRegistry_Unregister(t *testing.T) {
	r := NewDeviceRegistry()
	s := fakes.NewFakeMessageSender()
	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, s)

	r.Unregister("user-1", "dev-tv", s)
	assert.Empty(t, r.List("user-1"))
	// Idempotent - unknown device / already-removed is a no-op.
	r.Unregister("user-1", "dev-tv", s)
	r.Unregister("user-unknown", "whatever", nil)
}

// A stale disconnect must not evict a newer connection for the same
// deviceID. Unregister only removes the entry when the sender matches.
func TestDeviceRegistry_UnregisterStaleSenderNoOp(t *testing.T) {
	r := NewDeviceRegistry()
	stale := fakes.NewFakeMessageSender()
	fresh := fakes.NewFakeMessageSender()

	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, stale)
	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, fresh) // reconnect

	// The stale connection's defer fires late - it must not remove the
	// fresh entry.
	r.Unregister("user-1", "dev-tv", stale)
	require.Len(t, r.List("user-1"), 1, "fresh connection survives a stale disconnect")

	r.Unregister("user-1", "dev-tv", fresh)
	assert.Empty(t, r.List("user-1"))
}

func TestDeviceRegistry_SendCommand(t *testing.T) {
	r := NewDeviceRegistry()
	s := fakes.NewFakeMessageSender()
	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, s)

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
	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())

	// Wrong device, wrong user, and a user with no devices at all.
	require.ErrorIs(t, r.SendCommand("user-1", "dev-missing", syncplay.DeviceCommand{Type: "play"}), ErrDeviceNotConnected)
	assert.ErrorIs(t, r.SendCommand("user-2", "dev-tv", syncplay.DeviceCommand{Type: "play"}), ErrDeviceNotConnected)
}

func TestDeviceRegistry_SendCommand_SenderError(t *testing.T) {
	r := NewDeviceRegistry()
	s := fakes.NewFakeMessageSender()
	s.Err = errors.New("socket closed")
	_, _ = r.Register("user-1", "dev-tv", "TV", syncplay.DeviceTypeTV, s)

	err := r.SendCommand("user-1", "dev-tv", syncplay.DeviceCommand{Type: "pause"})
	require.Error(t, err)
	assert.NotErrorIs(t, err, ErrDeviceNotConnected, "a send failure is distinct from not-connected")
}

func TestDeviceRegistry_QuotaRejectsNewDevicePastCap(t *testing.T) {
	r := NewDeviceRegistry()
	for i := range MaxDevicesPerUser {
		dev, err := r.Register("user-1", fmt.Sprintf("dev-%d", i), "D", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())
		require.NoError(t, err)
		require.Equal(t, fmt.Sprintf("dev-%d", i), dev.ID)
	}
	require.Len(t, r.List("user-1"), MaxDevicesPerUser)

	// A brand-new deviceID past the cap is rejected and never stored.
	_, err := r.Register("user-1", "dev-overflow", "D", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())
	require.ErrorIs(t, err, ErrTooManyDevices)
	assert.Len(t, r.List("user-1"), MaxDevicesPerUser, "rejected device must not be stored")

	// The cap is per-user: another user is unaffected.
	_, err = r.Register("user-2", "dev-x", "D", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())
	require.NoError(t, err)
}

func TestDeviceRegistry_QuotaAllowsReconnectAtCap(t *testing.T) {
	r := NewDeviceRegistry()
	var lastID string
	for i := range MaxDevicesPerUser {
		lastID = fmt.Sprintf("dev-%d", i)
		_, err := r.Register("user-1", lastID, "D", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())
		require.NoError(t, err)
	}
	// Reconnecting an EXISTING deviceID at the cap must succeed (it replaces
	// the entry in place) rather than being rejected as overflow.
	fresh := fakes.NewFakeMessageSender()
	_, err := r.Register("user-1", lastID, "D", syncplay.DeviceTypeTV, fresh)
	require.NoError(t, err)
	require.Len(t, r.List("user-1"), MaxDevicesPerUser)
	require.NoError(t, r.SendCommand("user-1", lastID, syncplay.DeviceCommand{Type: syncplay.DeviceCmdPause}))
	assert.Len(t, fresh.Received(), 1, "reconnect replaced the sender")
}

func TestDeviceRegistry_QuotaFreedAfterUnregister(t *testing.T) {
	r := NewDeviceRegistry()
	for i := range MaxDevicesPerUser {
		_, err := r.Register("user-1", fmt.Sprintf("dev-%d", i), "D", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())
		require.NoError(t, err)
	}
	// Freeing a slot lets a new device register again.
	r.Unregister("user-1", "dev-0", nil)
	_, err := r.Register("user-1", "dev-new", "D", syncplay.DeviceTypeTV, fakes.NewFakeMessageSender())
	require.NoError(t, err)
	assert.Len(t, r.List("user-1"), MaxDevicesPerUser)
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
	for w := range workers {
		go func(w int) {
			defer wg.Done()
			uid := "user-1"
			did := "dev-" + string(rune('A'+w))
			for range iters {
				s := fakes.NewFakeMessageSender()
				_, _ = r.Register(uid, did, "D", syncplay.DeviceTypeTV, s)
				_ = r.SendCommand(uid, did, syncplay.DeviceCommand{Type: "play"})
				_ = r.List(uid)
				r.Unregister(uid, did, s)
			}
		}(w)
	}
	wg.Wait()
}

type countingSender struct {
	mu   sync.Mutex
	got  [][]byte
	fail bool
}

func (s *countingSender) SendMessage(data []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.fail {
		return errors.New("socket gone")
	}
	s.got = append(s.got, data)
	return nil
}

func (s *countingSender) frames() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]string, 0, len(s.got))
	for _, f := range s.got {
		out = append(out, string(f))
	}
	return out
}

func TestDeviceRegistry_BroadcastReachesEveryClientOfTheUserOnly(t *testing.T) {
	r := NewDeviceRegistry()
	tv, web, dead, other := &countingSender{}, &countingSender{}, &countingSender{fail: true}, &countingSender{}
	if _, err := r.Register("usr-1", "tv", "TV", "tv", tv); err != nil {
		t.Fatal(err)
	}
	if _, err := r.RegisterListener("usr-1", "web", "Browser", "web", web); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("usr-1", "dead", "Phone", "mobile", dead); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Register("usr-2", "tv", "TV", "tv", other); err != nil {
		t.Fatal(err)
	}

	sent := r.Broadcast("usr-1", syncplay.DeviceCommand{
		Type: syncplay.DeviceCmdSettingsChanged, Scope: syncplay.SettingsScopeProfile, ProfileID: "prf-1",
	})
	if sent != 2 {
		t.Errorf("sent = %d, want 2 (the dead socket is skipped)", sent)
	}
	want := `{"type":"settingsChanged","scope":"profile","profileId":"prf-1"}`
	for name, s := range map[string]*countingSender{"tv": tv, "web": web} {
		if f := s.frames(); len(f) != 1 || f[0] != want {
			t.Errorf("%s got %v, want one %s", name, f, want)
		}
	}
	if f := other.frames(); len(f) != 0 {
		t.Errorf("another user's device heard the change: %v", f)
	}
}

func TestDeviceRegistry_ListenerIsNotAPlayOnTarget(t *testing.T) {
	r := NewDeviceRegistry()
	web := &countingSender{}
	if _, err := r.RegisterListener("usr-1", "web", "Browser", "web", web); err != nil {
		t.Fatal(err)
	}
	if got := r.List("usr-1"); len(got) != 0 {
		t.Errorf("a listener is offered as a Play On target: %v", got)
	}
	err := r.SendCommand("usr-1", "web", syncplay.DeviceCommand{Type: syncplay.DeviceCmdPlay})
	if !errors.Is(err, ErrDeviceNotConnected) {
		t.Errorf("SendCommand to a listener = %v, want ErrDeviceNotConnected", err)
	}
}
