package user

import "testing"

// A parental restriction is what PIN-gates leaving the profile: kids always,
// a grown-up profile only when parental is on, configured, AND has a PIN -
// without the PIN the filter would be bypassable by a plain switch.
func TestProfile_IsParentalRestricted(t *testing.T) {
	var none *Profile
	if none.IsParentalRestricted() {
		t.Fatal("nil profile is not restricted")
	}
	if !(&Profile{IsKids: true}).IsParentalRestricted() {
		t.Fatal("a kids profile is always restricted, PIN or not")
	}
	cases := []struct {
		name     string
		parental *ParentalSettings
		want     bool
	}{
		{"no parental block", nil, false},
		{"disabled", &ParentalSettings{Enabled: false, MaxAge: new(12), PINHash: "h"}, false},
		{"enabled, nothing configured", &ParentalSettings{Enabled: true, PINHash: "h"}, false},
		{"enabled, max age, no PIN", &ParentalSettings{Enabled: true, MaxAge: new(12)}, false},
		{"enabled, max age, PIN", &ParentalSettings{Enabled: true, MaxAge: new(12), PINHash: "h"}, true},
		{"enabled, blocked tags, PIN", &ParentalSettings{Enabled: true, BlockedTags: []string{"horror"}, PINHash: "h"}, true},
	}
	for _, tc := range cases {
		p := &Profile{Parental: tc.parental}
		if got := p.IsParentalRestricted(); got != tc.want {
			t.Errorf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}

// Device quota: unset or zero means unlimited; a set limit is exceeded once
// the active count reaches it (each play session is one device).
func TestUser_DeviceQuota(t *testing.T) {
	u := &User{}
	if u.HasDeviceLimit() || u.DeviceQuotaExceeded(1000) {
		t.Fatal("no limit set = unlimited")
	}
	u.MaxDevicesPerProfile = new(0)
	if u.HasDeviceLimit() || u.DeviceQuotaExceeded(1000) {
		t.Fatal("zero = unlimited")
	}
	u.MaxDevicesPerProfile = new(2)
	if !u.HasDeviceLimit() || u.DeviceQuotaExceeded(1) {
		t.Fatal("one device under a limit of two must be allowed")
	}
	if !u.DeviceQuotaExceeded(2) || !u.DeviceQuotaExceeded(3) {
		t.Fatal("at or past the limit the next device is refused")
	}
}
