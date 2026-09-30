package network

import (
	"net"
	"slices"
	"sync"
)

// LiveDetector is a Detector that follows a LAN list which can change while
// the server runs (Admin -> Remote Access). The list is read on every call and
// the Detector rebuilt only when it differs, so the gate on every request
// parses CIDRs once per change, not once per request.
type LiveDetector struct {
	cidrs func() []string

	mu   sync.RWMutex
	last []string
	det  *Detector
}

// NewLiveDetector reads the LAN list through cidrs, which must be safe to call
// concurrently (a config read under its lock).
func NewLiveDetector(cidrs func() []string) *LiveDetector {
	return &LiveDetector{cidrs: cidrs}
}

// Detector returns the Detector for the list as it is now.
func (l *LiveDetector) Detector() *Detector {
	cur := l.cidrs()
	l.mu.RLock()
	det, same := l.det, l.det != nil && slices.Equal(cur, l.last)
	l.mu.RUnlock()
	if same {
		return det
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.det == nil || !slices.Equal(cur, l.last) {
		l.det = NewDetector(cur)
		l.last = slices.Clone(cur)
	}
	return l.det
}

// LiveCIDRSet is a CIDR membership test on a list that can change while the
// server runs (the trusted proxies). Same caching as LiveDetector: the list is
// read on every call and parsed again only when it differs. Invalid entries
// are skipped; lists are validated where they are written.
type LiveCIDRSet struct {
	cidrs func() []string

	mu   sync.RWMutex
	last []string
	nets []*net.IPNet
	init bool
}

// NewLiveCIDRSet reads the list through cidrs, which must be safe to call
// concurrently.
func NewLiveCIDRSet(cidrs func() []string) *LiveCIDRSet {
	return &LiveCIDRSet{cidrs: cidrs}
}

// Contains reports whether ip is inside the list as it is now.
func (l *LiveCIDRSet) Contains(ip net.IP) bool {
	if ip == nil {
		return false
	}
	for _, n := range l.current() {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

func (l *LiveCIDRSet) current() []*net.IPNet {
	cur := l.cidrs()
	l.mu.RLock()
	nets, same := l.nets, l.init && slices.Equal(cur, l.last)
	l.mu.RUnlock()
	if same {
		return nets
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if !l.init || !slices.Equal(cur, l.last) {
		parsed := make([]*net.IPNet, 0, len(cur))
		for _, c := range cur {
			if _, n, err := net.ParseCIDR(c); err == nil {
				parsed = append(parsed, n)
			}
		}
		l.nets, l.last, l.init = parsed, slices.Clone(cur), true
	}
	return l.nets
}
