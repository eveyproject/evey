package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"testing"

	extmkv "github.com/gravity-zero/mkvgo/mkv"
)

// A damaged region a subtitle read passed over is named in the log with the
// file and where the lost cues sat.
func TestSubtitleReadOptions_NamesTheDamagedRegion(t *testing.T) {
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	opts := subtitleReadOptions(context.Background(), "/media/a.mkv")
	if len(opts) != 1 || opts[0].OnSkip == nil {
		t.Fatal("no OnSkip: a damaged region would pass unnoticed")
	}
	opts[0].OnSkip(extmkv.DamagedRange{StartOffset: 1260397864, EndOffset: 1272589017, ApproxStartMs: 2833497, ApproxEndMs: 2864153})
	var line map[string]any
	if err := json.Unmarshal(buf.Bytes(), &line); err != nil {
		t.Fatalf("%v: %s", err, buf.String())
	}
	want := map[string]any{"level": "WARN", "path": "/media/a.mkv", "start_offset": 1260397864.0, "end_offset": 1272589017.0,
		"approx_start_ms": 2833497.0, "approx_end_ms": 2864153.0}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("%s = %v, want %v", k, line[k], v)
		}
	}
}

// On a real file damaged in its middle, a PGS track is delivered whole but
// for the cues inside the region, and the region is named once.
//
//	EVEY_DAMAGED_MKV=/path/to/file.mkv EVEY_DAMAGED_PGS_TRACK=11 go test ./internal/adapters/driving/http/ -run TestExtractPGS_PassesOverADamagedRegion -v
func TestExtractPGS_PassesOverADamagedRegion(t *testing.T) {
	path := os.Getenv("EVEY_DAMAGED_MKV")
	if path == "" {
		t.Skip("set EVEY_DAMAGED_MKV (and EVEY_DAMAGED_PGS_TRACK) to a file damaged in its middle")
	}
	track := uint64(11)
	if v := os.Getenv("EVEY_DAMAGED_PGS_TRACK"); v != "" {
		n, err := strconv.ParseUint(v, 10, 64)
		if err != nil {
			t.Fatal(err)
		}
		track = n
	}
	var buf bytes.Buffer
	prev := slog.Default()
	slog.SetDefault(slog.New(slog.NewJSONHandler(&buf, nil)))
	t.Cleanup(func() { slog.SetDefault(prev) })

	blob, err := extractPGSBlob(context.Background(), path, track, nil)
	if err != nil {
		t.Fatalf("extraction refused: %v", err)
	}
	h, err := decodePGSBlobHeader(blob)
	if err != nil {
		t.Fatal(err)
	}
	if n := strings.Count(buf.String(), "passed over a damaged region"); n != 1 {
		t.Fatalf("region named %d times, want once: %s", n, buf.String())
	}
	t.Logf("%d cues delivered; %s", len(h.Cues), strings.TrimSpace(buf.String()))
}
