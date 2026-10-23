package plugin_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The source serves the acknowledged zip on the first GET and another one
// afterwards: ApplyUpdate must install the bytes it hashed, never re-fetch.
func TestApplyUpdate_InstallsTheHashedBytesNotARefetch(t *testing.T) {
	zipA, zipB := []byte("zip-A-acknowledged"), []byte("zip-B-swapped")
	var gets atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if gets.Add(1) == 1 {
			_, _ = w.Write(zipA)
			return
		}
		_, _ = w.Write(zipB)
	}))
	defer srv.Close()

	svc, _, _, parser, _, _ := newTestInstallerService()
	ctx := context.Background()
	parser.Result = goManifestVersioned("toctou", "1.0.0")
	p, err := svc.InstallPlugin(ctx, tmpZip(t, "v1"))
	require.NoError(t, err)

	parser.Result = goManifestVersioned("toctou", "2.0.0")
	sumA := sha256.Sum256(zipA)
	hashA := hex.EncodeToString(sumA[:])
	p2, err := svc.ApplyUpdate(ctx, p.ID, srv.URL, hashA, false)
	require.NoError(t, err)
	assert.Equal(t, hashA, p2.SourceHash, "installed bytes differ from the hashed ones")
	assert.Equal(t, int32(1), gets.Load(), "candidate fetched more than once")
}
