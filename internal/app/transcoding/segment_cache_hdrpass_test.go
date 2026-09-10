package transcoding

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

// HEVC Main10 with the HDR kept is different bytes from the tone-mapped
// transcode: it forks the cache, conditionally so every SDR entry survives.
func TestDecisionFingerprint_Hash_DistinguishesHDRPassthrough(t *testing.T) {
	sdr := minimalFingerprint(t)
	hdr := minimalFingerprint(t)
	hdr.HDRPassthrough = true
	assert.NotEqual(t, sdr.Hash(), hdr.Hash())

	withTransfer := minimalFingerprint(t)
	withTransfer.HDRTransfer = "pq"
	assert.Equal(t, sdr.Hash(), withTransfer.Hash(), "the transfer is derived from the source, not a hash dimension")
}
