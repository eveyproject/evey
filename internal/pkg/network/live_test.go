package network

import (
	"net"
	"testing"
)

func TestLiveDetectorFollowsTheList(t *testing.T) {
	t.Parallel()
	lan := []string{"10.0.0.0/8"}
	live := NewLiveDetector(func() []string { return lan })
	ip := net.ParseIP("203.0.113.7")

	if got := live.Detector().DetectIP(ip); got != WAN {
		t.Fatalf("before the change: got %s, want wan", got)
	}
	first := live.Detector()
	if live.Detector() != first {
		t.Fatal("an unchanged list rebuilt the detector")
	}

	lan = []string{"10.0.0.0/8", "203.0.113.0/24"}
	if got := live.Detector().DetectIP(ip); got != LAN {
		t.Fatalf("after adding the range: got %s, want lan", got)
	}
}
