package httpapi

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

func clearZipCRCCache() {
	zipCRCCache.Lock()
	clear(zipCRCCache.m)
	zipCRCCache.Unlock()
}

func zipStreamFixture(t *testing.T) ([]archiveEntry, map[string][]byte) {
	t.Helper()
	dir := t.TempDir()
	big := bytes.Repeat([]byte("0123456789abcdef"), 300_000) // 4.8 MB, several reads
	files := map[string][]byte{
		"Saison 01/E01 - Épisode.mkv": big,
		"Saison 01/E02 - Vide.mkv":    {},
		"Saison 02/E01 - Court.mkv":   []byte("short"),
	}
	var entries []archiveEntry
	for i, name := range []string{"Saison 01/E01 - Épisode.mkv", "Saison 01/E02 - Vide.mkv", "Saison 02/E01 - Court.mkv"} {
		p := filepath.Join(dir, fmt.Sprintf("f%d.mkv", i))
		if err := os.WriteFile(p, files[name], 0o644); err != nil {
			t.Fatal(err)
		}
		entries = append(entries, archiveEntry{path: p, name: name})
	}
	return entries, files
}

func getArchive(t *testing.T, entries []archiveEntry, method string, header map[string]string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, "/x.zip", nil)
	for k, v := range header {
		req.Header.Set(k, v)
	}
	rec := httptest.NewRecorder()
	serveMediaArchive(rec, req, "My Show", entries, "http://x")
	return rec
}

// checkZip opens the archive and reads every entry to its end, which is where
// archive/zip verifies the CRC.
func checkZip(t *testing.T, body []byte, want map[string][]byte) {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	if len(zr.File) != len(want) {
		t.Fatalf("entries = %d, want %d", len(zr.File), len(want))
	}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("%s: open: %v", f.Name, err)
		}
		got, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("%s: read: %v", f.Name, err)
		}
		if !bytes.Equal(got, want[f.Name]) {
			t.Errorf("%s: content differs (%d bytes, want %d)", f.Name, len(got), len(want[f.Name]))
		}
	}
}

func TestZipStream_FullArchiveAnnouncesItsLengthAndChecksOut(t *testing.T) {
	clearZipCRCCache()
	entries, files := zipStreamFixture(t)
	rec := getArchive(t, entries, http.MethodGet, nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d", rec.Code)
	}
	body := rec.Body.Bytes()
	if cl := rec.Header().Get("Content-Length"); cl != strconv.Itoa(len(body)) {
		t.Fatalf("Content-Length = %q, body = %d bytes", cl, len(body))
	}
	if rec.Header().Get("Accept-Ranges") != "bytes" {
		t.Errorf("Accept-Ranges = %q, want bytes", rec.Header().Get("Accept-Ranges"))
	}
	checkZip(t, body, files)

	head := getArchive(t, entries, http.MethodHead, nil)
	if head.Header().Get("Content-Length") != strconv.Itoa(len(body)) || head.Body.Len() != 0 {
		t.Errorf("HEAD: Content-Length %q, body %d", head.Header().Get("Content-Length"), head.Body.Len())
	}
}

// A transfer cut anywhere - in a header, in the data, in a descriptor, in the
// central directory - resumes into the very same bytes, even after a restart
// emptied the CRC cache.
func TestZipStream_ResumeFromAnyOffsetRebuildsTheSameArchive(t *testing.T) {
	clearZipCRCCache()
	entries, files := zipStreamFixture(t)
	full := getArchive(t, entries, http.MethodGet, nil).Body.Bytes()
	etag := getArchive(t, entries, http.MethodHead, nil).Header().Get("ETag")
	layout, _ := newZipLayout(entries)
	f0 := layout.files[0]
	cuts := map[string]int64{
		"in the first header":       10,
		"in the first file's data":  f0.data + 1_000_003,
		"in the first descriptor":   f0.data + f0.size + 5,
		"in the empty file's entry": layout.files[1].header + 7,
		"in the central directory":  layout.central + 13,
	}
	for where, cut := range cuts {
		clearZipCRCCache()
		rec := getArchive(t, entries, http.MethodGet, map[string]string{"Range": fmt.Sprintf("bytes=%d-", cut), "If-Range": etag})
		if rec.Code != http.StatusPartialContent {
			t.Fatalf("%s: code = %d, want 206", where, rec.Code)
		}
		resumed := append(append([]byte{}, full[:cut]...), rec.Body.Bytes()...)
		if !bytes.Equal(resumed, full) {
			t.Fatalf("%s: resumed archive differs from the full one", where)
		}
		checkZip(t, resumed, files)
	}
}

// A download manager fetching the tail first needs every CRC before any data.
func TestZipStream_TailFirstWithAColdCache(t *testing.T) {
	clearZipCRCCache()
	entries, _ := zipStreamFixture(t)
	full := getArchive(t, entries, http.MethodGet, nil).Body.Bytes()
	clearZipCRCCache()
	rec := getArchive(t, entries, http.MethodGet, map[string]string{"Range": "bytes=-300"})
	if rec.Code != http.StatusPartialContent {
		t.Fatalf("code = %d, want 206", rec.Code)
	}
	if !bytes.Equal(rec.Body.Bytes(), full[len(full)-300:]) {
		t.Fatal("tail differs from the full archive's")
	}
}

// A file replaced between two attempts changes the ETag: If-Range then gets
// the whole new archive, never a splice of two.
func TestZipStream_ChangedFileRestartsTheTransfer(t *testing.T) {
	clearZipCRCCache()
	entries, _ := zipStreamFixture(t)
	etag := getArchive(t, entries, http.MethodHead, nil).Header().Get("ETag")
	later := time.Now().Add(time.Hour)
	if err := os.WriteFile(entries[2].path, []byte("longer now"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(entries[2].path, later, later); err != nil {
		t.Fatal(err)
	}
	rec := getArchive(t, entries, http.MethodGet, map[string]string{"Range": "bytes=100-", "If-Range": etag})
	if rec.Code != http.StatusOK {
		t.Fatalf("code = %d, want 200 (stale If-Range)", rec.Code)
	}
	if rec.Header().Get("ETag") == etag {
		t.Error("ETag did not change with the file")
	}
}
