package config

import "testing"

func TestRemoteBitrateCapKbpsTakesTheStricterOfTheTwoCeilings(t *testing.T) {
	cases := []struct{ transcoding, remoteAccess, want int }{
		{0, 0, 0},
		{0, 8000, 8000},
		{20000, 0, 20000},
		{20000, 8000, 8000},
		{8000, 20000, 8000},
		{-1, 5000, 5000},
	}
	for _, c := range cases {
		if got := RemoteBitrateCapKbps(c.transcoding, c.remoteAccess); got != c.want {
			t.Errorf("RemoteBitrateCapKbps(%d, %d) = %d, want %d", c.transcoding, c.remoteAccess, got, c.want)
		}
	}
}

// The served master reads this accessor: a cap set only in Remote Access must
// reach it, as one set only in Transcoding does.
func TestEffectiveRemoteCeilingHonoursBothSettings(t *testing.T) {
	c := &Config{}
	c.RemoteAccess.MaxRemoteBitrateKbps = 8000
	if got := c.EffectiveMaxVideoBitrateRemoteKbps(); got != 8000 {
		t.Fatalf("Remote Access cap alone: %d, want 8000", got)
	}
	c.RemoteAccess.MaxRemoteBitrateKbps = 0
	c.Transcoding.MaxVideoBitrateRemoteKbps = 20000
	if got := c.EffectiveMaxVideoBitrateRemoteKbps(); got != 20000 {
		t.Fatalf("Transcoding ceiling alone: %d, want 20000", got)
	}
}
