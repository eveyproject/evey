package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	domainlibrary "github.com/evey-project/evey/internal/domain/library"
)

// A file stored in one-track blocks is refused with the shed contract every
// client already renders (header + body code), never served to stall on.
func TestCheckPlayable_RefusesChunkedFileWithTypedShed(t *testing.T) {
	w := httptest.NewRecorder()
	item := &domainlibrary.MediaItem{File: &domainlibrary.MediaFile{Path: "/m/a.mkv", InterleaveBackstepMs: 366_000}}
	assert.False(t, checkPlayable(w, item))
	assert.Equal(t, http.StatusServiceUnavailable, w.Code)
	assert.Equal(t, ErrCodeFileNeedsRemux, w.Header().Get("X-Evey-Error"))
	assert.Contains(t, w.Body.String(), `"code":"file_needs_remux"`)

	w = httptest.NewRecorder()
	item.File.InterleaveBackstepMs = domainlibrary.InterleaveBackstepFlagMs // at the floor: playable
	assert.True(t, checkPlayable(w, item))
	assert.True(t, checkPlayable(httptest.NewRecorder(), &domainlibrary.MediaItem{}), "no file, nothing to refuse here")
}
