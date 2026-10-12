package httpapi

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	domainlibrary "github.com/eveyproject/evey/internal/domain/library"
)

func archiveTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func zipEntryNames(t *testing.T, body []byte) []string {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(body), int64(len(body)))
	if err != nil {
		t.Fatalf("zip read: %v", err)
	}
	names := make([]string, 0, len(zr.File))
	for _, f := range zr.File {
		names = append(names, f.Name)
		if f.Method != zip.Store {
			t.Errorf("entry %q method = %d, want Store (0) for already-compressed media", f.Name, f.Method)
		}
	}
	return names
}

func runArchiveDownload(t *testing.T, getter *fakeMediaItemGetter, id string) *httptest.ResponseRecorder {
	t.Helper()
	h := downloadHandler(
		getter,
		"http://localhost",
		&fakeLibraryAccessChecker{allowed: true}, nil)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/media/"+id+"/download", nil)
	req.SetPathValue("id", id)
	req = withHTTPClaims(req, testClaims(false))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

func TestDownloadHandler_Season_ZIP(t *testing.T) {
	dir := t.TempDir()
	ep1 := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Pilot", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e1.mkv", "one")}}
	ep2 := &domainlibrary.MediaItem{ID: "ep2", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Second", IndexNumber: new(2), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e2.mkv", "two")}}
	season := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	// children returned unsorted to exercise index-number ordering.
	getter := &fakeMediaItemGetter{item: season, children: map[string][]*domainlibrary.MediaItem{"s1": {ep2, ep1}}}

	w := runArchiveDownload(t, getter, "s1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/zip" {
		t.Errorf("Content-Type = %q, want application/zip", ct)
	}
	if cd := w.Header().Get("Content-Disposition"); cd == "" || !bytes.Contains([]byte(cd), []byte("Season 1.zip")) {
		t.Errorf("Content-Disposition = %q, want a Season 1.zip attachment", cd)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"E01 - Pilot.mkv", "E02 - Second.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

func TestDownloadHandler_Series_ZIP_NestedBySeason(t *testing.T) {
	dir := t.TempDir()
	ep1 := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Pilot", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e1.mkv", "one")}}
	ep2 := &domainlibrary.MediaItem{ID: "ep2", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Finale", IndexNumber: new(2), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e2.mkv", "two")}}
	s1 := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	s2 := &domainlibrary.MediaItem{ID: "s2", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 2", IndexNumber: new(2)}
	series := &domainlibrary.MediaItem{ID: "sr1", LibraryID: "lib1", Type: domainlibrary.Series, Title: "My Show"}
	getter := &fakeMediaItemGetter{item: series, children: map[string][]*domainlibrary.MediaItem{
		"sr1": {s2, s1}, // unsorted
		"s1":  {ep1},
		"s2":  {ep2},
	}}

	w := runArchiveDownload(t, getter, "sr1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (%s)", w.Code, w.Body.String())
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"Season 01/E01 - Pilot.mkv", "Season 02/E02 - Finale.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("zip entries = %v, want %v", got, want)
	}
}

func TestDownloadHandler_Season_NoFiles_NotFound(t *testing.T) {
	season := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Empty"}
	getter := &fakeMediaItemGetter{item: season}
	w := runArchiveDownload(t, getter, "s1")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for season with no episode files, got %d", w.Code)
	}
}

func TestDownloadHandler_CrossLibraryEpisode_Skipped(t *testing.T) {
	dir := t.TempDir()
	inLib := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Local", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e1.mkv", "one")}}
	foreign := &domainlibrary.MediaItem{ID: "ep2", LibraryID: "lib2", Type: domainlibrary.Episode, Title: "Foreign", IndexNumber: new(2), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, dir, "e2.mkv", "two")}}
	season := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	getter := &fakeMediaItemGetter{item: season, children: map[string][]*domainlibrary.MediaItem{"s1": {inLib, foreign}}}

	w := runArchiveDownload(t, getter, "s1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"E01 - Local.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("cross-library episode should be excluded: got %v, want %v", got, want)
	}
}

func TestDownloadHandler_Season_PathOutsideLibraryRoot_Skipped(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir() // a path no library is configured to serve
	legit := &domainlibrary.MediaItem{ID: "ep1", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Legit", IndexNumber: new(1), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, root, "e1.mkv", "ok")}}
	// Simulates a poisoned media_files.path pointing at a real but out-of-root
	// file (e.g. a secret). It exists on disk and is openable, yet must NOT be
	// bundled because it sits outside every configured library root.
	poisoned := &domainlibrary.MediaItem{ID: "ep2", LibraryID: "lib1", Type: domainlibrary.Episode, Title: "Secret", IndexNumber: new(2), File: &domainlibrary.MediaFile{Path: archiveTempFile(t, elsewhere, "jwt.key", "secret")}}
	season := &domainlibrary.MediaItem{ID: "s1", LibraryID: "lib1", Type: domainlibrary.Season, Title: "Season 1", IndexNumber: new(1)}
	getter := &fakeMediaItemGetter{
		item:      season,
		children:  map[string][]*domainlibrary.MediaItem{"s1": {legit, poisoned}},
		libraries: []*domainlibrary.Library{{Paths: []string{root}}},
	}

	w := runArchiveDownload(t, getter, "s1")
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	got := zipEntryNames(t, w.Body.Bytes())
	want := []string{"E01 - Legit.mkv"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("path outside library roots must be excluded: got %v, want %v", got, want)
	}
}

func TestDownloadHandler_MovieNoFile_NotFound(t *testing.T) {
	movie := &domainlibrary.MediaItem{ID: "m1", LibraryID: "lib1", Type: domainlibrary.MovieItem, Title: "Orphan"}
	getter := &fakeMediaItemGetter{item: movie}
	w := runArchiveDownload(t, getter, "m1")
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 for movie with no file, got %d", w.Code)
	}
}
