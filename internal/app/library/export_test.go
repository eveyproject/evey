package library

import "time"

// SetTrickplaySettleForTest shortens the completeness check's size probe and
// the delay before a postponed generation is tried again.
func (s *TrickplayService) SetTrickplaySettleForTest(probe, retry time.Duration) {
	s.settle = trickplaySettle{probe: probe, retry: retry}
}
