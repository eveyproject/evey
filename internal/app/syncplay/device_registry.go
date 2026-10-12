package syncplay

import (
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"sync"
	"time"

	"github.com/eveyproject/evey/internal/domain/syncplay"
)

// ErrDeviceNotConnected is returned by SendCommand when the target device
// has no live WebSocket - e.g. the TV app was closed since the web UI
// last listed devices.
var ErrDeviceNotConnected = errors.New("device not connected")

// ErrTooManyDevices is returned by Register when a user already holds the
// maximum number of registered devices and tries to add a *new* one.
var ErrTooManyDevices = errors.New("too many registered devices for user")

// MaxDevicesPerUser caps the live device entries (each = one WebSocket
// sender held in memory) a single user can register, bounding the memory a
// misbehaving or malicious authenticated client can pin.
const MaxDevicesPerUser = 50

type deviceEntry struct {
	device syncplay.Device
	sender syncplay.MessageSender
}

// DeviceRegistry tracks first-party clients that have registered as Play
// On targets. It's the sibling of Hub: Hub relays commands between members
// of a sync room (peer-to-peer), DeviceRegistry pushes commands from the
// server to a single addressed device (server-to-client). Both ride the
// same persistent-WebSocket + MessageSender machinery.
type DeviceRegistry struct {
	mu      sync.RWMutex
	devices map[string]map[string]*deviceEntry // userID → deviceID → entry
}

func NewDeviceRegistry() *DeviceRegistry {
	return &DeviceRegistry{
		devices: make(map[string]map[string]*deviceEntry),
	}
}

// Register adds (or replaces, on reconnect) a controllable device for a
// user. deviceID is client-generated and stable per install, so a
// reconnect with the same ID cleanly supersedes the stale connection.
func (r *DeviceRegistry) Register(userID, deviceID, name, deviceType string, sender syncplay.MessageSender) (syncplay.Device, error) {
	dev := syncplay.Device{
		ID:          deviceID,
		UserID:      userID,
		Name:        name,
		Type:        deviceType,
		ConnectedAt: time.Now().UTC(),
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.devices[userID] == nil {
		r.devices[userID] = make(map[string]*deviceEntry)
	}
	byUser := r.devices[userID]
	// A reconnect reuses an existing deviceID: it replaces the stale entry in
	// place and never grows the count, so it's always allowed even at the
	// cap. Only a brand-new deviceID past the cap is rejected. The check and
	// the insert share the lock so the count can't be raced past the cap.
	if _, exists := byUser[deviceID]; !exists && len(byUser) >= MaxDevicesPerUser {
		return syncplay.Device{}, ErrTooManyDevices
	}
	byUser[deviceID] = &deviceEntry{device: dev, sender: sender}
	return dev, nil
}

// Unregister removes a device. Safe to call for an unknown device. Only
// removes the entry if the sender matches - protects against a stale
// disconnect racing a fresh reconnect from clobbering the new connection.
func (r *DeviceRegistry) Unregister(userID, deviceID string, sender syncplay.MessageSender) {
	r.mu.Lock()
	defer r.mu.Unlock()
	byUser := r.devices[userID]
	if byUser == nil {
		return
	}
	if e, ok := byUser[deviceID]; ok && (sender == nil || e.sender == sender) {
		delete(byUser, deviceID)
	}
	if len(byUser) == 0 {
		delete(r.devices, userID)
	}
}

// List returns the user's currently-connected devices, sorted by name for
// a stable UI ordering.
func (r *DeviceRegistry) List(userID string) []syncplay.Device {
	r.mu.RLock()
	byUser := r.devices[userID]
	out := make([]syncplay.Device, 0, len(byUser))
	for _, e := range byUser {
		out = append(out, e.device)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool {
		if out[i].Name != out[j].Name {
			return out[i].Name < out[j].Name
		}
		return out[i].ID < out[j].ID
	})
	return out
}

// SendCommand pushes a playback command to one of the user's devices.
// Returns ErrDeviceNotConnected if the device isn't currently registered
// - the caller (HTTP handler) maps that to a 404 so the UI can refresh
// its device list.
func (r *DeviceRegistry) SendCommand(userID, deviceID string, cmd syncplay.DeviceCommand) error {
	r.mu.RLock()
	var sender syncplay.MessageSender
	if byUser := r.devices[userID]; byUser != nil {
		if e, ok := byUser[deviceID]; ok {
			sender = e.sender
		}
	}
	r.mu.RUnlock()
	if sender == nil {
		return ErrDeviceNotConnected
	}
	data, err := json.Marshal(cmd)
	if err != nil {
		return fmt.Errorf("marshal device command: %w", err)
	}
	if err := sender.SendMessage(data); err != nil {
		return fmt.Errorf("send device command: %w", err)
	}
	return nil
}
