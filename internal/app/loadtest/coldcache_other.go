//go:build !linux

package loadtest

import "errors"

// evictFromCache is Linux-only: nowhere else does Evey ship a server, and a
// silent no-op here would let a run claim "cold cache" while every byte still
// came from RAM. The runner logs the failure and reports the run as warm.
func evictFromCache(string) error {
	return errors.New("cold cache eviction requires Linux (posix_fadvise)")
}
