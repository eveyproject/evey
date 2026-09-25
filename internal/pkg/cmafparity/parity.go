// Package cmafparity checks the CMAF fragments an encoder produced against
// the timeline a manifest declared for them, with mkvgo as the reader: the
// spans mkvgo's DASHFromCMAF derives from the fragments' own moof boxes are
// compared, tick for tick, with what Evey declared from its plan. It is the
// oracle behind the plan-declared DASH manifest: the manifest is written
// before any fragment exists, so nothing but the fragments can prove it.
package cmafparity

import (
	"context"
	"encoding/xml"
	"fmt"
	"math"
	"math/big"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/gravity-zero/mkvgo/mp4"
)

// Declared is the timeline a manifest promised for one representation:
// Timescale, and the start and duration of segment n in that timescale.
type Declared struct {
	Timescale int64
	Start     func(n int) int64
	Dur       func(n int) int64
	// Count is how many segments the manifest lists; a fragment numbered
	// past it is not addressed by any player (the muxer flushes a trailing
	// audio-only fragment when the audio outlasts the picture) and is
	// reported as Extra, not compared. 0: every fragment is compared.
	Count int
}

// Mismatch is one segment whose fragment disagrees with its declaration.
type Mismatch struct {
	Index                         int
	DeclaredStart, DeclaredDur    int64
	FileStart, FileDur            int64
	DeclaredTimescale, FileTimesc int64
}

func (m Mismatch) String() string {
	return fmt.Sprintf("segment %d: declared %d+%d @%d, fragment %d+%d @%d",
		m.Index, m.DeclaredStart, m.DeclaredDur, m.DeclaredTimescale, m.FileStart, m.FileDur, m.FileTimesc)
}

// Result is what Verify found.
type Result struct {
	// Indices are the segments present on disk, in order; every one was checked.
	Indices []int
	// FileTimescale is the track timescale the fragments are written in.
	FileTimescale int64
	Mismatches    []Mismatch
	// Extra are fragments numbered past the declaration's Count.
	Extra []int
}

// OK reports whether every present fragment matched its declaration.
func (r Result) OK() bool { return len(r.Mismatches) == 0 }

// Verify reads the fragments named prefix_NNN.m4s (with prefix_init.mp4) in
// dir through mkvgo and compares each one's real span with the declaration
// for its index. toleranceSec 0 demands tick-exact equality (cross-multiplied,
// no float); a positive tolerance allows that much slack either way - what a
// separate audio rendition needs, since the muxer cuts it on the first AAC
// frame past each boundary. audio says the representation carries no video.
// A separate audio rendition is read alone (mkvgo 0.33: an audio-only
// presentation).
func Verify(ctx context.Context, dir, prefix string, audio bool, d Declared, toleranceSec float64) (Result, error) {
	var res Result
	if d.Timescale <= 0 || d.Start == nil || d.Dur == nil {
		return res, fmt.Errorf("cmafparity: incomplete declaration")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return res, fmt.Errorf("cmafparity: %w", err)
	}
	type seg struct {
		idx  int
		name string
	}
	var segs []seg
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() || !strings.HasPrefix(n, prefix+"_") || !strings.HasSuffix(n, ".m4s") {
			continue
		}
		body := strings.TrimSuffix(strings.TrimPrefix(n, prefix+"_"), ".m4s")
		idx, perr := strconv.Atoi(body)
		if perr != nil {
			continue
		}
		segs = append(segs, seg{idx, n})
	}
	if len(segs) == 0 {
		return res, fmt.Errorf("cmafparity: no %s_NNN.m4s under %s", prefix, dir)
	}
	sort.Slice(segs, func(i, j int) bool { return segs[i].idx < segs[j].idx })
	rep := mp4.CMAFRepresentation{Dir: dir, Init: prefix + "_init.mp4"}
	for _, s := range segs {
		rep.Segments = append(rep.Segments, s.name)
		res.Indices = append(res.Indices, s.idx)
	}
	var p mp4.CMAFPresentation
	contentType := "video"
	if audio {
		p.Audio = []mp4.CMAFRepresentation{rep}
		contentType = "audio"
	} else {
		p.Video = []mp4.CMAFRepresentation{rep}
	}
	mpd, err := mp4.DASHFromCMAF(ctx, p)
	if err != nil {
		return res, fmt.Errorf("cmafparity: mkvgo: %w", err)
	}
	timescale, spans, err := parseTimeline(mpd, contentType)
	if err != nil {
		return res, err
	}
	if len(spans) != len(segs) {
		return res, fmt.Errorf("cmafparity: mkvgo described %d segments for %d fragments", len(spans), len(segs))
	}
	res.FileTimescale = timescale
	for i, s := range segs {
		if d.Count > 0 && s.idx >= d.Count {
			res.Extra = append(res.Extra, s.idx)
			continue
		}
		ds, dd := d.Start(s.idx), d.Dur(s.idx)
		fs, fd := spans[i][0], spans[i][1]
		if !same(ds, d.Timescale, fs, timescale, toleranceSec) || !same(dd, d.Timescale, fd, timescale, toleranceSec) {
			res.Mismatches = append(res.Mismatches, Mismatch{
				Index: s.idx, DeclaredStart: ds, DeclaredDur: dd, FileStart: fs, FileDur: fd,
				DeclaredTimescale: d.Timescale, FileTimesc: timescale,
			})
		}
	}
	return res, nil
}

// same compares a/tsA with b/tsB: exactly (cross-multiplied in big ints)
// when tolerance is 0, else within tolerance seconds.
func same(a, tsA, b, tsB int64, toleranceSec float64) bool {
	if toleranceSec <= 0 {
		l := new(big.Int).Mul(big.NewInt(a), big.NewInt(tsB))
		r := new(big.Int).Mul(big.NewInt(b), big.NewInt(tsA))
		return l.Cmp(r) == 0
	}
	return math.Abs(float64(a)/float64(tsA)-float64(b)/float64(tsB)) <= toleranceSec
}

// The MPD shape DASHFromCMAF emits for one representation: a SegmentTemplate
// (counting names) or a SegmentList (any other naming), each with a
// SegmentTimeline.
type mpdDoc struct {
	Periods []struct {
		Sets []struct {
			ContentType string `xml:"contentType,attr"`
			Reps        []struct {
				Template *segmentAddressing `xml:"SegmentTemplate"`
				List     *segmentAddressing `xml:"SegmentList"`
			} `xml:"Representation"`
		} `xml:"AdaptationSet"`
	} `xml:"Period"`
}

type segmentAddressing struct {
	Timescale int64 `xml:"timescale,attr"`
	Timeline  struct {
		S []struct {
			T *int64 `xml:"t,attr"`
			D int64  `xml:"d,attr"`
			R int    `xml:"r,attr"`
		} `xml:"S"`
	} `xml:"SegmentTimeline"`
}

// parseTimeline returns the timescale and the (start, duration) of every
// segment the manifest lists for the first representation of contentType,
// repeats expanded.
func parseTimeline(mpd []byte, contentType string) (int64, [][2]int64, error) {
	var doc mpdDoc
	if err := xml.Unmarshal(mpd, &doc); err != nil {
		return 0, nil, fmt.Errorf("cmafparity: parse mpd: %w", err)
	}
	for _, p := range doc.Periods {
		for _, set := range p.Sets {
			if set.ContentType != contentType {
				continue
			}
			for _, rep := range set.Reps {
				addr := rep.Template
				if addr == nil {
					addr = rep.List
				}
				if addr == nil {
					continue
				}
				var out [][2]int64
				var t int64
				for _, s := range addr.Timeline.S {
					if s.T != nil {
						t = *s.T
					}
					for range s.R + 1 {
						out = append(out, [2]int64{t, s.D})
						t += s.D
					}
				}
				return addr.Timescale, out, nil
			}
		}
	}
	return 0, nil, fmt.Errorf("cmafparity: no %s representation in mpd", contentType)
}
