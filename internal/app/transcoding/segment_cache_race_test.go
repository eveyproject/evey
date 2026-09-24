package transcoding

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestTouch_ConcurrentWithSweep stresses the access-time registry under -race:
// many goroutines Touch/LastAccess while the LRU sweeper concurrently lists
// usage and cleans up hashes. Proves the mutex discipline added for fix (b)
// (Touch/LastAccess/CleanupHash + listHashUsage merge all guard accessTimes).
func TestTouch_ConcurrentWithSweep(t *testing.T) {
	c, _ := newTestCache(t)
	for i := range 8 {
		writeSegments(t, c, fmt.Sprintf("h%d", i), 2, 100)
	}
	// Aggressive sweeper: 8×200B = 1600B over a 500B cap with a 1ms interval, so
	// CleanupHash runs constantly against the Touch/LastAccess traffic.
	stop := NewCacheBudget(c).Start(EvictionPolicy{
		MaxBytes:      500,
		SweepInterval: time.Millisecond,
		MinFreeBytes:  -1,
		DownloadTTL:   -1,
		ProtectWindow: time.Nanosecond,
	}, quietLogger())
	defer stop()

	var wg sync.WaitGroup
	for range 4 {
		wg.Go(func() {
			for i := range 300 {
				h := fmt.Sprintf("h%d", i%8)
				c.Touch(h)
				_, _ = c.LastAccess(h)
			}
		})
	}
	wg.Wait()
}
