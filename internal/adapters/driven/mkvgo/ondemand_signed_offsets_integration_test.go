package mkvgo

// A B-frame video is packaged with signed composition offsets: no edit list
// takes a composition shift back, a frame reordered ahead of its keyframe
// carries a negative offset instead. A player that ignores the video edit
// list (Chrome's MSE, Media3's DASH, once the list opens with an empty edit)
// then presents the picture on time rather than late by the reordering depth.
//
// OPT-IN, same as the other packager integration tests:
//
//	EVEY_TRANSCODE_ITEST=1 go test ./internal/adapters/driven/mkvgo/ -run SignedOffsetsIntegration -v

import (
	"context"
	"encoding/binary"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
)

func TestOnDemandPackager_SignedOffsetsIntegration(t *testing.T) {
	if os.Getenv("EVEY_TRANSCODE_ITEST") != "1" {
		t.Skip("integration test: set EVEY_TRANSCODE_ITEST=1 (use `make transcode-itest`)")
	}
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("ffmpeg not in PATH")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	dir := t.TempDir()
	src := filepath.Join(dir, "bframes.mkv")
	gen := exec.CommandContext(ctx, "ffmpeg", "-hide_banner", "-v", "error", "-y",
		"-f", "lavfi", "-i", "testsrc2=size=320x180:rate=24",
		"-t", "12",
		"-c:v", "libx264", "-bf", "3", "-b-pyramid", "normal", "-g", "48", "-keyint_min", "48", "-pix_fmt", "yuv420p",
		src)
	if out, err := gen.CombinedOutput(); err != nil {
		t.Fatalf("ffmpeg source generation failed: %v\n%s", err, out)
	}

	for _, tc := range []struct {
		name string
		plan func() (planResource, error)
	}{
		{"single source", func() (planResource, error) {
			pkg := NewOnDemandPackager()
			t.Cleanup(pkg.Close)
			pl, err := pkg.Plan(ctx, src, apptranscoding.OnDemandOptions{SegmentMs: 6000})
			return pl, err
		}},
		{"chain", func() (planResource, error) {
			pkg := NewConcatPackager()
			t.Cleanup(pkg.Close)
			pl, err := pkg.Plan(ctx, []string{src, src}, apptranscoding.ConcatOptions{SegmentMs: 6000})
			return pl, err
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			plan, err := tc.plan()
			if err != nil {
				t.Fatalf("Plan: %v", err)
			}
			init, _, err := plan.Resource(ctx, initName(t, ctx, plan))
			if err != nil {
				t.Fatalf("Resource(init): %v", err)
			}
			for _, mt := range videoEditMediaTimes(init) {
				if mt > 0 {
					t.Errorf("video edit list takes back a composition shift (media_time %d), want none", mt)
				}
			}
			seg, _, err := plan.Resource(ctx, firstVideoSegment(t, ctx, plan))
			if err != nil {
				t.Fatalf("Resource(segment): %v", err)
			}
			if !hasNegativeCompositionOffset(seg) {
				t.Error("no negative composition offset in the first video segment, want signed offsets on a B-frame source")
			}
		})
	}
}

type planResource interface {
	Resource(context.Context, string) ([]byte, string, error)
}

// videoPlaylistLines returns the non-tag lines of the video media playlist and
// its EXT-X-MAP URI.
func videoPlaylistLines(t *testing.T, ctx context.Context, plan planResource) (mapURI string, segs []string) {
	t.Helper()
	pl, _, err := plan.Resource(ctx, "playlist.m3u8")
	if err != nil {
		t.Fatalf("Resource(playlist.m3u8): %v", err)
	}
	for line := range strings.SplitSeq(strings.ReplaceAll(string(pl), "\r", ""), "\n") {
		switch {
		case strings.HasPrefix(line, `#EXT-X-MAP:URI="`):
			if mapURI == "" {
				mapURI, _, _ = strings.Cut(strings.TrimPrefix(line, `#EXT-X-MAP:URI="`), `"`)
			}
		case line != "" && line[0] != '#':
			segs = append(segs, line)
		}
	}
	if mapURI == "" || len(segs) == 0 {
		t.Fatalf("video playlist has no EXT-X-MAP or no segment:\n%s", pl)
	}
	return mapURI, segs
}

func initName(t *testing.T, ctx context.Context, plan planResource) string {
	m, _ := videoPlaylistLines(t, ctx, plan)
	return m
}

func firstVideoSegment(t *testing.T, ctx context.Context, plan planResource) string {
	_, segs := videoPlaylistLines(t, ctx, plan)
	return segs[0]
}

// mp4Box is one ISO BMFF box: its type and payload.
type mp4Box struct {
	typ     string
	payload []byte
}

func mp4Boxes(b []byte) []mp4Box {
	var out []mp4Box
	for len(b) >= 8 {
		size := int(binary.BigEndian.Uint32(b))
		typ := string(b[4:8])
		hdr := 8
		if size == 1 && len(b) >= 16 {
			size = int(binary.BigEndian.Uint64(b[8:]))
			hdr = 16
		}
		if size == 0 {
			size = len(b)
		}
		if size < hdr || size > len(b) {
			break
		}
		out = append(out, mp4Box{typ, b[hdr:size]})
		b = b[size:]
	}
	return out
}

func mp4Child(b []byte, typ string) []byte {
	for _, x := range mp4Boxes(b) {
		if x.typ == typ {
			return x.payload
		}
	}
	return nil
}

// videoEditMediaTimes lists the media_time of every edit in the init's video
// track edit list (an empty edit is -1).
func videoEditMediaTimes(init []byte) []int64 {
	var out []int64
	for _, trak := range mp4Boxes(mp4Child(init, "moov")) {
		if trak.typ != "trak" {
			continue
		}
		hdlr := mp4Child(mp4Child(trak.payload, "mdia"), "hdlr")
		if len(hdlr) < 12 || string(hdlr[8:12]) != "vide" {
			continue
		}
		elst := mp4Child(mp4Child(trak.payload, "edts"), "elst")
		if len(elst) < 8 {
			continue
		}
		version, n := elst[0], int(binary.BigEndian.Uint32(elst[4:]))
		p := elst[8:]
		for range n {
			if version == 1 && len(p) >= 20 {
				out = append(out, int64(binary.BigEndian.Uint64(p[8:])))
				p = p[20:]
			} else if version == 0 && len(p) >= 12 {
				out = append(out, int64(int32(binary.BigEndian.Uint32(p[4:]))))
				p = p[12:]
			}
		}
	}
	return out
}

// hasNegativeCompositionOffset reports whether a trun of the segment carries a
// negative (version 1, signed) sample composition time offset.
func hasNegativeCompositionOffset(seg []byte) bool {
	for _, moof := range mp4Boxes(seg) {
		if moof.typ != "moof" {
			continue
		}
		for _, traf := range mp4Boxes(moof.payload) {
			if traf.typ != "traf" {
				continue
			}
			for _, x := range mp4Boxes(traf.payload) {
				if x.typ != "trun" || len(x.payload) < 8 || x.payload[0] != 1 {
					continue
				}
				flags := binary.BigEndian.Uint32(x.payload) & 0xffffff
				if flags&0x800 == 0 {
					continue
				}
				n := int(binary.BigEndian.Uint32(x.payload[4:]))
				p := x.payload[8:]
				if flags&0x1 != 0 {
					p = p[4:]
				}
				if flags&0x4 != 0 {
					p = p[4:]
				}
				for range n {
					for _, f := range []uint32{0x100, 0x200, 0x400} {
						if flags&f != 0 {
							p = p[4:]
						}
					}
					if len(p) < 4 {
						return false
					}
					if int32(binary.BigEndian.Uint32(p)) < 0 {
						return true
					}
					p = p[4:]
				}
			}
		}
	}
	return false
}
