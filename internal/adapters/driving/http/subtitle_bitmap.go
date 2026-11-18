package httpapi

// subtitle_bitmap.go - the VobSub (DVD) and DVB bitmap tracks, served in the
// same decoded-image blob as PGS (subtitle_pgs.go) so every player draws them
// with the code it already has.
//
// mkvgo reads and assembles the cues: an embedded track from the subtitle
// index when there is one (matroska.ForEachSubtitleBitmap[From]), a sidecar
// .sup or .idx/.sub pair from the file beside the video
// (matroska.ForEachSubtitleBitmapFile). A damaged region or a unit that does
// not decode costs its own pictures, not the track.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/gravity-zero/mkvgo/matroska"
	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

// mkvgo's names (mkv.Track.Codec) for the bitmap formats decoded here; the
// Matroska codec IDs are S_VOBSUB and S_DVBSUB.
const (
	codecVobSub = "vobsub"
	codecDVBSub = "dvbsub"
)

// forEachMatroskaBitmapCue calls fn with each cue of an embedded VobSub or DVB
// track, from the subtitle index when there is one. mkvgo passes over a damaged
// region and leaves out a unit that does not decode; a track where every unit
// failed is an error (bitmapSkipTally), so the failure is remembered instead of
// an empty track being served.
func forEachMatroskaBitmapCue(ctx context.Context, filePath string, trackID uint64, ix *matroska.SubtitleIndex, fn func(matroska.BitmapCue) error, opts ...matroska.Options) error {
	var tally bitmapSkipTally
	opts = tally.wrap(opts)
	fn = tally.count(fn)
	var err error
	if ix != nil {
		err = matroska.ForEachSubtitleBitmapFrom(ctx, filePath, trackID, ix, fn, opts...)
	} else {
		err = matroska.ForEachSubtitleBitmap(ctx, filePath, trackID, fn, opts...)
	}
	if err != nil {
		return err
	}
	return tally.err()
}

// bitmapSkipTally counts the cues delivered and the units mkvgo left out
// because they did not decode (Options.OnSkip with Err set).
type bitmapSkipTally struct {
	cues, failed int
	lastErr      error
}

// wrap chains the tally ahead of the caller's OnSkip.
func (t *bitmapSkipTally) wrap(opts []matroska.Options) []matroska.Options {
	var o matroska.Options
	if len(opts) > 0 {
		o = opts[0]
	}
	next := o.OnSkip
	o.OnSkip = func(r extmkv.DamagedRange) {
		if r.Err != nil {
			t.failed++
			t.lastErr = r.Err
		}
		if next != nil {
			next(r)
		}
	}
	return []matroska.Options{o}
}

func (t *bitmapSkipTally) count(fn func(matroska.BitmapCue) error) func(matroska.BitmapCue) error {
	return func(c matroska.BitmapCue) error {
		t.cues++
		return fn(c)
	}
}

// err is the last decode error when no cue came out and some unit failed.
func (t *bitmapSkipTally) err() error {
	if t.cues == 0 && t.failed > 0 {
		return t.lastErr
	}
	return nil
}

// bitmapServable reports whether a bitmap track can be served as decoded
// images rather than burnt in: PGS, VobSub and DVB. XSUB cannot.
//
// ⚠️ Must agree with isClientDrawableSubtitle in
// frontend/packages/api/src/imageSubtitles.ts.
func bitmapServable(codec string) bool {
	switch strings.TrimSpace(strings.ToLower(codec)) {
	case "hdmv_pgs_subtitle", "pgssub", "pgs",
		"dvd_subtitle", "dvdsub",
		"dvb_subtitle", "dvbsub":
		return true
	}
	return false
}

// isPGSCodec reports whether mkvgo's codec name (mkv.Track.Codec) is PGS, read
// by mkvgo's PGS walk (extractPGSBlob).
func isPGSCodec(codec string) bool {
	return codec == "pgs"
}

// isExternalBitmapSubtitle reports whether a sidecar file is a picture
// subtitle: a PGS .sup, or the .idx of a VobSub .idx/.sub pair.
func isExternalBitmapSubtitle(ext string) bool {
	return ext == ".sup" || ext == ".idx"
}

// extractExternalBitmapBlob decodes a sidecar picture subtitle into the same
// blob as an embedded track. subPath is the .sub of a VobSub pair, unused for a
// .sup. Of a .idx holding several languages, the first is read: the scanner
// names the file by one language, and a pair made for one film rarely holds
// more.
func extractExternalBitmapBlob(ctx context.Context, path, subPath string) ([]byte, error) {
	if !isExternalBitmapSubtitle(strings.ToLower(filepath.Ext(path))) {
		return nil, fmt.Errorf("%s is not a picture subtitle file", filepath.Base(path))
	}
	var b bitmapBlobBuilder
	var tally bitmapSkipTally
	opts := tally.wrap([]matroska.Options{{
		OnSkip: func(r extmkv.DamagedRange) { logSubtitleDamage(ctx, path, r) },
		// mkvgo looks for the .sub under the case of the .idx; vobSubPairSub
		// already found it whatever its case.
		FS: &extmkv.FS{Open: func(p string) (extmkv.ReadSeekCloser, error) {
			if subPath != "" && strings.EqualFold(filepath.Ext(p), ".sub") {
				p = subPath
			}
			return os.Open(p) //nolint:gosec // resolved under the video's directory by safeJoin
		}},
	}})
	if err := matroska.ForEachSubtitleBitmapFile(ctx, path, 0, tally.count(b.add), opts...); err != nil {
		return nil, err
	}
	if err := tally.err(); err != nil {
		return nil, err
	}
	return b.blob()
}

// vobSubPairSub finds the .sub beside an .idx, whatever the case of its
// extension.
func vobSubPairSub(idxPath string) (string, bool) {
	dir, name := filepath.Split(idxPath)
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	if dir == "" {
		dir = "."
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	// The scanner pairs the two in any letter case (hasSibling); so does this.
	for _, e := range entries {
		n := e.Name()
		if !strings.EqualFold(filepath.Ext(n), ".sub") || strings.TrimSuffix(n, filepath.Ext(n)) != stem {
			continue
		}
		// Followed through a symbolic link, as the scanner lists it and as
		// the video itself is served.
		if st, err := os.Stat(filepath.Join(dir, n)); err == nil && st.Mode().IsRegular() {
			return filepath.Join(dir, n), true
		}
	}
	return "", false
}

// externalBitmapKey keys a sidecar's blob on every file it is decoded from: a
// VobSub's .sub holds the pictures, so replacing it alone must miss the cache.
func externalBitmapKey(cache subtitleCache, path, subPath string) string {
	k := cache.keyForKind(path, 0, subtitleKindPGS)
	if k == "" || subPath == "" {
		return k
	}
	ks := cache.keyFor(subPath, 0)
	if ks == "" {
		return ""
	}
	h := sha256.Sum256([]byte(k + "\x00" + ks))
	return string(subtitleKindPGS) + "-" + hex.EncodeToString(h[:])
}

// serveExternalBitmapSubtitle answers the subtitle route for a sidecar picture
// subtitle with the decoded-image blob, cached like an embedded track's: a
// full film's .sup takes seconds to decode, not worth paying per request.
func serveExternalBitmapSubtitle(w http.ResponseWriter, r *http.Request, cache subtitleCache, path, baseURL string) {
	var subPath string
	if strings.EqualFold(filepath.Ext(path), ".idx") {
		p, ok := vobSubPairSub(path)
		if !ok {
			writeSubtitleFailed(w, r, baseURL, "the .idx has no .sub beside it")
			return
		}
		subPath = p
	}
	key := externalBitmapKey(cache, path, subPath)
	if data, ok := cache.get(key); ok {
		writePGSBlob(w, r, data)
		return
	}
	if reason, failed := cache.getFailure(key); failed {
		writeSubtitleFailed(w, r, baseURL, reason)
		return
	}
	// One decode however many viewers or retries ask at once, remembered when
	// it fails, as an embedded track's extraction is.
	work := func(ctx context.Context) ([]byte, error) {
		if data, ok := cache.get(key); ok {
			return data, nil
		}
		data, err := extractExternalBitmapBlob(ctx, path, subPath)
		if err != nil {
			if fErr := cache.putFailure(key, err); fErr != nil {
				slog.WarnContext(ctx, "subtitle: external picture subtitle failure memo store failed",
					slog.String("path", path), slog.Any("err", fErr))
			}
			return nil, err
		}
		if cErr := cache.put(key, data); cErr != nil {
			slog.WarnContext(ctx, "subtitle: external picture subtitle cache store failed",
				slog.String("path", path), slog.Any("err", cErr))
		}
		return data, nil
	}
	data, err := runSharedSubtitleExtraction(r.Context(), subtitleFlightKey(key, path), path, work)
	if err != nil {
		slog.WarnContext(r.Context(), "subtitle: external picture subtitle decode failed",
			slog.String("path", path), slog.Any("err", err))
		writeSubtitleFailed(w, r, baseURL, err.Error())
		return
	}
	writePGSBlob(w, r, data)
}
