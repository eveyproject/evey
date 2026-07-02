package mkvgo

import (
	"testing"

	"github.com/stretchr/testify/assert"

	apptranscoding "github.com/evey-project/evey/internal/app/transcoding"
)

// The plan cache must not alias plans that produce different bytes. Encrypted
// segments depend on the exact AES key, so two keys for the same source (e.g.
// after a secret rotation) must yield distinct cache keys.
func TestOnDemandCacheKey_DistinguishesAESKeys(t *testing.T) {
	src := "/media/movie.mkv" // stat fails → size/mtime 0; we only assert on the key-tag difference
	plain := onDemandCacheKey(src, apptranscoding.OnDemandOptions{SegmentMs: 6000, URLPrefix: "od_"})
	encA := onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("aaaaaaaaaaaaaaaa"), AESKeyURI: "key",
	})
	encB := onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("bbbbbbbbbbbbbbbb"), AESKeyURI: "key",
	})

	assert.NotEqual(t, plain, encA, "encrypted and plaintext plans must not alias")
	assert.NotEqual(t, encA, encB, "different AES keys for the same source must not alias")

	// Determinism: same inputs → same key (cache hit).
	assert.Equal(t, encA, onDemandCacheKey(src, apptranscoding.OnDemandOptions{
		SegmentMs: 6000, URLPrefix: "od_", AESKey: []byte("aaaaaaaaaaaaaaaa"), AESKeyURI: "key",
	}), "same options → same cache key")
}
