package httpapi

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	appmetadata "github.com/evey-project/evey/internal/app/metadata"
	domainmeta "github.com/evey-project/evey/internal/domain/metadata"
	metafakes "github.com/evey-project/evey/internal/domain/metadata/fakes"
	sharedfakes "github.com/evey-project/evey/internal/domain/shared/fakes"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The two pure decisions in the upload path, kept honest here because both are
// driven by client-supplied data and both fail open if written carelessly.

func TestArtworkTypeFromForm(t *testing.T) {
	for _, tc := range []struct {
		in     string
		want   string
		wantOK bool
	}{
		{"poster", "ARTWORK_TYPE_PRIMARY", true},
		{"primary", "ARTWORK_TYPE_PRIMARY", true},
		{"backdrop", "ARTWORK_TYPE_BACKDROP", true},
		{"BACKDROP", "ARTWORK_TYPE_BACKDROP", true},
		{"  Poster  ", "ARTWORK_TYPE_PRIMARY", true},
		// Absent means poster: it is the field an admin edits by default, and
		// the alternative is failing an upload over a missing optional field.
		{"", "ARTWORK_TYPE_PRIMARY", true},
		// Anything else is refused rather than stored: a record with a type no
		// screen reads is invisible, so the admin would see the upload
		// "succeed" and nothing change.
		{"logo", "", false},
		{"ARTWORK_TYPE_PRIMARY; DROP", "", false},
	} {
		got, ok := artworkTypeFromForm(tc.in)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("artworkTypeFromForm(%q) = (%q,%v), want (%q,%v)", tc.in, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestImageExtFromContentType(t *testing.T) {
	for _, tc := range []struct {
		ct     string
		want   string
		wantOK bool
	}{
		{"image/jpeg", ".jpg", true},
		{"image/png", ".png", true},
		{"image/webp", ".webp", true},
		// DetectContentType appends a charset on some types; a prefix match
		// must still work.
		{"image/png; charset=utf-8", ".png", true},

		// The refusals are the point of the function.
		//
		// SVG is an image by MIME type and a script container in practice: it
		// would be stored under our own origin and served back to browsers.
		{"image/svg+xml", "", false},
		{"image/gif", "", false},
		{"text/html; charset=utf-8", "", false},
		{"application/octet-stream", "", false},
		{"application/x-msdownload", "", false},
		{"", "", false},
	} {
		got, ok := imageExtFromContentType(tc.ct)
		if got != tc.want || ok != tc.wantOK {
			t.Errorf("imageExtFromContentType(%q) = (%q,%v), want (%q,%v)", tc.ct, got, ok, tc.want, tc.wantOK)
		}
	}
}

// --- POST /api/v1/admin/media/{id}/artwork (registerArtworkUploadRoute) ---
//
// registerArtworkUploadRoute wires its own admin middleware and registers
// straight onto the mux (unlike the other artwork handlers, it doesn't hand
// back a bare http.HandlerFunc), so these tests go through a real
// http.ServeMux end to end rather than calling an inner handler directly.

// newArtworkCmdService builds a real *appmetadata.CommandService (the
// concrete type registerArtworkUploadRoute requires) backed by fakes, with
// dataDir set so AddCustomArtwork can write to tmp.
func newArtworkCmdService(t *testing.T, dataDir string) *appmetadata.CommandService {
	t.Helper()
	cmdSvc := appmetadata.NewCommandService(
		metafakes.NewFakeMetadataOverrideRepository(),
		metafakes.NewFakeArtworkRepository(),
		metafakes.NewFakeArtworkDownloader(),
		sharedfakes.NewFakeEventBus(),
		nil,
	)
	appmetadata.WithAutoDownload(dataDir, false)(cmdSvc)
	return cmdSvc
}

func newArtworkUploadMux(cmdSvc *appmetadata.CommandService, verifier TokenVerifier, mediaGetter MediaItemGetter) *http.ServeMux {
	mux := http.NewServeMux()
	registerArtworkUploadRoute(mux, cmdSvc, verifier, mediaGetter, &fakeArtworkRepo{records: map[string][]*domainmeta.ArtworkRecord{}})
	return mux
}

// newArtworkMultipartRequest builds a multipart POST to the upload route.
// declaredContentType is the Content-Type the client attaches to the file
// part - deliberately independent from body's actual bytes, so a test can
// prove the server decides by content, not by what the client claims.
func newArtworkMultipartRequest(t *testing.T, mediaID, formType, filename, declaredContentType string, body []byte) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if formType != "" {
		require.NoError(t, mw.WriteField("type", formType))
	}
	if body != nil {
		h := make(textproto.MIMEHeader)
		h.Set("Content-Disposition", `form-data; name="artwork"; filename="`+filename+`"`)
		if declaredContentType != "" {
			h.Set("Content-Type", declaredContentType)
		}
		fw, err := mw.CreatePart(h)
		require.NoError(t, err)
		_, err = fw.Write(body)
		require.NoError(t, err)
	}
	require.NoError(t, mw.Close())

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/"+mediaID+"/artwork", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestArtworkUpload_Success(t *testing.T) {
	dataDir := t.TempDir()
	cmdSvc := newArtworkCmdService(t, dataDir)
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	req := newArtworkMultipartRequest(t, "m1", "poster", "cover.png", "image/png", pngPixel)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	require.Equal(t, http.StatusCreated, w.Code, "body=%s", w.Body.String())
	assert.Contains(t, w.Body.String(), `"type":"ARTWORK_TYPE_PRIMARY"`)
	assert.Contains(t, w.Body.String(), `"id":`)
}

func TestArtworkUpload_NoAuth_401(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{err: errTest}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	req := newArtworkMultipartRequest(t, "m1", "poster", "cover.png", "image/png", pngPixel)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusUnauthorized, w.Code)
}

func TestArtworkUpload_NonAdmin_403(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(false)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	req := newArtworkMultipartRequest(t, "m1", "poster", "cover.png", "image/png", pngPixel)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusForbidden, w.Code)
}

func TestArtworkUpload_MissingMediaID_400(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	// A URL-encoded space is a non-empty path segment (so the mux pattern
	// still matches "{id}"), but TrimSpace collapses it to "" - exactly the
	// case the handler's own blank-id guard exists for.
	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/%20/artwork", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestArtworkUpload_MediaNotFound_404(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{err: errTest}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	req := httptest.NewRequest(http.MethodPost, "/api/v1/admin/media/ghost/artwork", nil)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestArtworkUpload_InvalidType_400(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	req := newArtworkMultipartRequest(t, "m1", "banner", "cover.png", "image/png", pngPixel)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code)
}

// This is the pin for the whole path: a body whose declared Content-Type is
// image/png but whose actual bytes are SVG must be refused. If the handler
// ever starts trusting the multipart part's Content-Type header (or the
// filename) instead of sniffing, this is the test that catches it.
func TestArtworkUpload_RejectsSVGDisguisedAsPNG(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	svg := []byte(`<svg xmlns="http://www.w3.org/2000/svg" width="1" height="1"><script>alert(1)</script></svg>`)
	req := newArtworkMultipartRequest(t, "m1", "poster", "cover.png", "image/png", svg)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}

func TestArtworkUpload_RejectsGIF(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	gif := []byte("GIF89a\x01\x00\x01\x00\x00\x00\x00")
	req := newArtworkMultipartRequest(t, "m1", "poster", "cover.gif", "image/gif", gif)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}

// A request body past maxArtworkUpload must be refused cleanly (400), not
// panic or 500. The oversized payload is one deterministic byte repeated -
// cheap to build and still well past the 13MB cap.
func TestArtworkUpload_TooLarge_400(t *testing.T) {
	cmdSvc := newArtworkCmdService(t, t.TempDir())
	verifier := &fakeTokenVerifier{claims: testClaims(true)}
	mediaGetter := &fakeMediaItemGetter{item: testMediaItem("lib1")}
	mux := newArtworkUploadMux(cmdSvc, verifier, mediaGetter)

	oversized := bytes.Repeat([]byte{0x89}, maxArtworkUpload+(64<<10))
	req := newArtworkMultipartRequest(t, "m1", "poster", "cover.png", "image/png", oversized)
	req.Header.Set("Authorization", "Bearer tok")
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	assert.Equal(t, http.StatusBadRequest, w.Code, "body=%s", w.Body.String())
}
