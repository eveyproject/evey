// intro-diag is a one-off diagnostic tool to inspect why pHash intro detection
// matched (or failed to match) on a given series. Reads trickplay sprites from
// disk and prints the frame-by-frame Hamming match map.
//
// Optional audio-confirmation pass: if --audio is provided with mediafile paths
// (target then siblings, in the same order as ID args), the tool extracts an
// RMS audio profile around each candidate trickplay window and runs a sliding
// Pearson cross-correlation against each sibling. Real intros yield correlation
// >0.6 across most siblings; coincidental scene transitions don't.
//
// Usage:
//
//	INTERVAL=7 CUTOFF=120 intro-diag <trickplay_root> <target_id> <sib_id...> \
//	    [--audio <target_path> <sib_path...>]
package main

import (
	"context"
	"fmt"
	"math"
	"math/bits"
	"os"
	"path/filepath"
	"strconv"

	ffmpegadapter "github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	_ "golang.org/x/image/webp"
)

const (
	tileW = 5
	tileH = 5
	pxW   = 320
	pxH   = 180
)

var (
	interval    = envIntDefault("INTERVAL", 15)
	cutoff      = envIntDefault("CUTOFF", 52)
	fmThreshold = envFloatDefault("THRESHOLD", 0.50)
)

func envFloatDefault(key string, def float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	return f
}

func envIntDefault(key string, def int) int {
	v := os.Getenv(key)
	if v == "" {
		return def
	}
	i, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return i
}

func loadSprites(dir string) []uint64 {
	det := ffmpegadapter.NewIntroDetector("", "")
	var hashes []uint64
	for n := 1; ; n++ {
		path := filepath.Join(dir, fmt.Sprintf("trickplay_%03d.webp", n))
		if _, err := os.Stat(path); err != nil {
			break
		}
		hs, err := det.ExtractTileHashes(path, tileW, tileH, pxW, pxH)
		if err != nil {
			break
		}
		hashes = append(hashes, hs...)
	}
	return hashes
}

func hamming(a, b uint64) int { return bits.OnesCount64(a ^ b) }

func fmtTime(sec int) string { return fmt.Sprintf("%02d:%02d", sec/60, sec%60) }

func main() {
	if len(os.Args) < 4 {
		fmt.Fprintln(os.Stderr, "usage: intro-diag <root> <target_id> <sib_id...> [--audio <target_path> <sib_path...>]")
		os.Exit(2)
	}
	args := os.Args[1:]
	var audioPaths []string
	if i := indexOf(args, "--audio"); i >= 0 {
		audioPaths = args[i+1:]
		args = args[:i]
	}
	if len(args) < 3 {
		fmt.Fprintln(os.Stderr, "need root + target + at least one sibling")
		os.Exit(2)
	}
	root := args[0]
	targetID := args[1]
	siblingIDs := args[2:]

	type epData struct {
		id     string
		hashes []uint64
	}

	load := func(id string) []uint64 {
		hs := loadSprites(filepath.Join(root, id))
		if len(hs) > cutoff {
			hs = hs[:cutoff]
		}
		return hs
	}

	target := load(targetID)
	fmt.Printf("INTERVAL=%ds CUTOFF=%d\n", interval, cutoff)
	fmt.Printf("TARGET %s : %d frames (covers 0 to %s)\n",
		targetID[:8], len(target), fmtTime(len(target)*interval))

	siblings := make([]epData, 0, len(siblingIDs))
	for _, id := range siblingIDs {
		hs := load(id)
		fmt.Printf("sib    %s : %d frames\n", id[:8], len(hs))
		siblings = append(siblings, epData{id, hs})
	}
	fmt.Println()

	// FrequencyMap (Hamming<=8, threshold=0.70)
	allEps := make([][]uint64, 0, 1+len(siblings))
	allEps = append(allEps, target)
	for _, s := range siblings {
		allEps = append(allEps, s.hashes)
	}
	required := int(float64(len(allEps))*fmThreshold + 0.999)
	fmt.Printf("FrequencyMap (Hamming<=8, threshold %.2f → required ≥%d/%d episodes)\n", fmThreshold, required, len(allEps))
	intro := make(map[uint64]bool)
	candSeen := make(map[uint64]bool)
	for _, ep := range allEps {
		for _, h := range ep {
			if candSeen[h] {
				continue
			}
			candSeen[h] = true
			cnt := 0
			for _, other := range allEps {
				for _, oh := range other {
					if hamming(h, oh) <= 8 {
						cnt++
						break
					}
				}
			}
			if cnt >= required {
				intro[h] = true
			}
		}
	}
	fmt.Printf("introHashes set size: %d\n\n", len(intro))

	matched := make([]bool, len(target))
	for i, h := range target {
		for fh := range intro {
			if hamming(h, fh) <= 8 {
				matched[i] = true
				break
			}
		}
	}

	// Match map
	fmt.Println("Target match map (# = matches an introHash, . = unique):")
	for i := 0; i < len(target); i += 20 {
		end := i + 20
		if end > len(target) {
			end = len(target)
		}
		row := make([]byte, 0, end-i)
		for j := i; j < end; j++ {
			if matched[j] {
				row = append(row, '#')
			} else {
				row = append(row, '.')
			}
		}
		fmt.Printf("  frames %3d-%-3d (%s-%s) : %s\n",
			i, end-1, fmtTime(i*interval), fmtTime((end-1)*interval), string(row))
	}

	// Window detection: gap-tolerant merge (gap≤2 frames = ≤2*interval seconds)
	fmt.Println("\nCandidate windows (gap-tolerant, gap≤2 frames):")
	type span struct{ start, end, matches int }
	type rawRun struct{ start, end int }
	var raw []rawRun
	rs, rl := 0, 0
	for i, m := range matched {
		if m {
			if rl == 0 {
				rs = i
			}
			rl++
		} else if rl > 0 {
			raw = append(raw, rawRun{rs, rs + rl})
			rl = 0
		}
	}
	if rl > 0 {
		raw = append(raw, rawRun{rs, rs + rl})
	}
	if len(raw) == 0 {
		fmt.Println("  (no runs)")
		return
	}
	merged := []span{{raw[0].start, raw[0].end, raw[0].end - raw[0].start}}
	for _, r := range raw[1:] {
		last := &merged[len(merged)-1]
		gap := r.start - last.end
		if gap <= 2 {
			last.end = r.end
			last.matches += r.end - r.start
		} else {
			merged = append(merged, span{r.start, r.end, r.end - r.start})
		}
	}

	// Filter: spans with at least 2 matches
	candidates := merged[:0]
	for _, s := range merged {
		if s.matches >= 2 {
			candidates = append(candidates, s)
		}
	}
	for _, s := range candidates {
		fmt.Printf("  span frames %d-%d (%s-%s) span=%d matches=%d\n",
			s.start, s.end-1,
			fmtTime(s.start*interval),
			fmtTime((s.end-1)*interval),
			s.end-s.start, s.matches)
	}

	if len(audioPaths) == 0 {
		fmt.Println("\n(skipping audio confirmation: pass --audio <target_path> <sib_paths...> to enable)")
		return
	}
	if len(audioPaths) != 1+len(siblings) {
		fmt.Fprintf(os.Stderr, "audio paths count mismatch: got %d, want %d\n", len(audioPaths), 1+len(siblings))
		os.Exit(2)
	}

	// Audio confirmation pass
	fmt.Println("\n=== AUDIO CONFIRMATION ===")
	fmt.Println("Method: extract PCM 8kHz mono around each candidate, compute RMS")
	fmt.Println("envelope (250ms bins), sliding Pearson correlation between target")
	fmt.Println("and each sibling. Score ≥0.6 = same audio signal.")

	det := ffmpegadapter.NewIntroDetector("ffmpeg", "ffprobe")
	ctx := context.Background()
	const padSec = 10.0
	const searchSec = 30.0 // sibling search window pad
	binMs := envIntDefault("BIN_MS", 250)

	for ci, c := range candidates {
		startSec := float64(c.start * interval)
		endSec := float64(c.end * interval)
		fmt.Printf("\n--- candidate #%d  trickplay window %s-%s (matches=%d) ---\n",
			ci+1, fmtTime(int(startSec)), fmtTime(int(endSec)), c.matches)

		tStart := startSec - padSec
		if tStart < 0 {
			tStart = 0
		}
		tDur := endSec + padSec - tStart
		fmt.Printf("  target audio range: %.1fs-%.1fs (%.1fs)\n", tStart, tStart+tDur, tDur)

		const numBands = 12
		targetFp, err := det.AudioBandedFingerprint(ctx, audioPaths[0], tStart, tDur, binMs)
		if err != nil {
			fmt.Printf("  target fingerprint extract error: %v\n", err)
			continue
		}
		nBins := len(targetFp) / numBands
		fmt.Printf("  target bins: %d (%.1fs window, %d bands/bin)\n",
			nBins, float64(nBins*binMs)/1000, numBands)

		type sibResult struct {
			id    string
			corr  float64
			drift float64
		}
		results := make([]sibResult, 0, len(siblings))
		for si, sib := range siblings {
			sStart := startSec - searchSec
			if sStart < 0 {
				sStart = 0
			}
			sDur := endSec + searchSec - sStart
			sibFp, err := det.AudioBandedFingerprint(ctx, audioPaths[1+si], sStart, sDur, binMs)
			if err != nil {
				fmt.Printf("  sib %s fingerprint error: %v\n", sib.id[:8], err)
				continue
			}
			corr, offBins := ffmpegadapter.SlidingBestPearsonBanded(targetFp, sibFp, numBands)
			offSec := float64(offBins*binMs) / 1000
			driftSec := offSec - (tStart - sStart) // 0 = perfect alignment
			results = append(results, sibResult{sib.id, corr, driftSec})
		}

		// Strong = corr >= 0.5
		const corrThreshold = 0.5
		strongDrifts := make([]float64, 0, len(results))
		for _, r := range results {
			if r.corr >= corrThreshold {
				strongDrifts = append(strongDrifts, r.drift)
			}
		}
		// Median of strong drifts
		var median float64
		if n := len(strongDrifts); n > 0 {
			sortedD := make([]float64, n)
			copy(sortedD, strongDrifts)
			for i := 1; i < n; i++ {
				for j := i; j > 0 && sortedD[j] < sortedD[j-1]; j-- {
					sortedD[j], sortedD[j-1] = sortedD[j-1], sortedD[j]
				}
			}
			median = sortedD[n/2]
		}

		// Print results with coherence markers
		coherent := 0
		const driftTolerance = 6.0
		for _, r := range results {
			marker := "  "
			coh := false
			if r.corr >= corrThreshold {
				if math.Abs(r.drift-median) <= driftTolerance {
					marker = "✓ "
					coh = true
					coherent++
				} else {
					marker = "~ " // strong corr but drift outlier
				}
			} else if r.corr >= 0.4 {
				marker = "· "
			}
			_ = coh
			fmt.Printf("  %ssib %s  corr=%.3f  drift=%+.1fs\n",
				marker, r.id[:8], r.corr, r.drift)
		}
		coherentRatio := float64(coherent) / float64(len(results))
		fmt.Printf("  drift_median=%+.1fs  coherent=%d/%d (%.0f%%)\n",
			median, coherent, len(results), coherentRatio*100)
		verdict := "REJECT"
		if coherentRatio >= 0.6 && coherent >= 3 {
			verdict = "CONFIRM (real intro at drift " + fmt.Sprintf("%+.1fs", median) + ")"
		} else if coherentRatio >= 0.3 {
			verdict = "AMBIGUOUS"
		}
		fmt.Printf("  >>> %s\n", verdict)
	}
}

func indexOf(s []string, v string) int {
	for i, x := range s {
		if x == v {
			return i
		}
	}
	return -1
}
