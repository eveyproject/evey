// trickplay-verify checks whether each trickplay tile actually shows the frame
// at the timestamp recorded in trickplay.json. For a sample of tiles, we
// re-extract the source frame at the recorded ms via ffmpeg and aHash it,
// then compare to the aHash of the trickplay tile. Low Hamming distance =
// frame matches the recorded timestamp.
//
// Usage: trickplay-verify <trickplay_dir> <source_mkv_path> [N_samples=5]
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"math/bits"
	"math/rand"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"

	ffmpegadapter "github.com/eveyproject/evey/internal/adapters/driven/ffmpeg"
	"golang.org/x/image/draw"
	_ "golang.org/x/image/webp"
)

const tileW, tileH = 5, 5
const pxW, pxH = 320, 180

type sidecar struct {
	TileWidth       int     `json:"tile_width"`
	TileHeight      int     `json:"tile_height"`
	FramesPerSprite int     `json:"frames_per_sprite"`
	FrameTimestamps []int64 `json:"frame_ms"`
}

func aHashGray(img image.Image) uint64 {
	const size = 8
	dst := image.NewGray(image.Rect(0, 0, size, size))
	draw.BiLinear.Scale(dst, dst.Bounds(), img, img.Bounds(), draw.Src, nil)
	var sum uint64
	for i := 0; i < size*size; i++ {
		sum += uint64(dst.Pix[i])
	}
	mean := sum / (size * size)
	var h uint64
	for i := 0; i < size*size; i++ {
		if uint64(dst.Pix[i]) > mean {
			h |= 1 << uint(i)
		}
	}
	return h
}

func extractSourceFrameHash(srcPath string, atSec float64) (uint64, error) {
	args := []string{"-nostdin", "-ss", strconv.FormatFloat(atSec, 'f', 3, 64),
		"-i", srcPath, "-frames:v", "1", "-vf", fmt.Sprintf("scale=%d:%d", pxW, pxH),
		"-c:v", "mjpeg", "-f", "image2pipe", "pipe:1"}
	cmd := exec.Command("ffmpeg", args...)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = nil
	if err := cmd.Run(); err != nil {
		return 0, fmt.Errorf("ffmpeg extract: %w", err)
	}
	img, _, err := image.Decode(&out)
	if err != nil {
		return 0, fmt.Errorf("decode: %w", err)
	}
	return aHashGray(img), nil
}

func main() {
	if len(os.Args) < 3 {
		fmt.Fprintln(os.Stderr, "usage: trickplay-verify <trickplay_dir> <source_mkv> [N=5]")
		os.Exit(2)
	}
	dir := os.Args[1]
	src := os.Args[2]
	n := 5
	if len(os.Args) >= 4 {
		if v, err := strconv.Atoi(os.Args[3]); err == nil {
			n = v
		}
	}

	scBytes, err := os.ReadFile(filepath.Join(dir, "trickplay.json"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "sidecar: %v\n", err)
		os.Exit(1)
	}
	var sc sidecar
	if err := json.Unmarshal(scBytes, &sc); err != nil {
		fmt.Fprintf(os.Stderr, "sidecar parse: %v\n", err)
		os.Exit(1)
	}
	total := len(sc.FrameTimestamps)
	if total < 10 {
		fmt.Fprintln(os.Stderr, "too few frames in sidecar")
		os.Exit(1)
	}

	det := ffmpegadapter.NewIntroDetector("ffmpeg", "ffprobe")

	// Build set of trickplay tile hashes (aHash 64 bits) by reading sprites.
	allTrickplayHashes := make([]uint64, 0, total)
	for sp := 1; ; sp++ {
		path := filepath.Join(dir, fmt.Sprintf("trickplay_%03d.webp", sp))
		if _, err := os.Stat(path); err != nil {
			break
		}
		hs, err := det.ExtractTileHashes(path, tileW, tileH, pxW, pxH)
		if err != nil {
			break
		}
		allTrickplayHashes = append(allTrickplayHashes, hs...)
		if len(allTrickplayHashes) >= total {
			allTrickplayHashes = allTrickplayHashes[:total]
			break
		}
	}
	if len(allTrickplayHashes) != total {
		fmt.Fprintf(os.Stderr, "extracted %d hashes, expected %d (sidecar)\n", len(allTrickplayHashes), total)
		// continue with what we have
	}

	rng := rand.New(rand.NewSource(42))
	type result struct {
		idx           int
		atSec         float64
		hammingMatch  int // hamming between source frame and trickplay tile (should be small)
		hammingMisalg int // hamming between source frame and ADJACENT trickplay tile (sanity, should be larger or comparable)
	}
	results := make([]result, 0, n)

	// Sample n distinct indices in the middle 80% to skip beginning/end.
	lo := total / 10
	hi := total - total/10
	if hi-lo < n {
		lo, hi = 0, total
	}
	picked := map[int]bool{}
	for len(picked) < n && len(picked) < hi-lo {
		i := lo + rng.Intn(hi-lo)
		if picked[i] {
			continue
		}
		picked[i] = true
		atSec := float64(sc.FrameTimestamps[i]) / 1000
		srcHash, err := extractSourceFrameHash(src, atSec)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  idx=%d t=%.3fs extract: %v\n", i, atSec, err)
			continue
		}
		match := bits.OnesCount64(srcHash ^ allTrickplayHashes[i])
		var misalg int
		if i+1 < len(allTrickplayHashes) {
			misalg = bits.OnesCount64(srcHash ^ allTrickplayHashes[i+1])
		}
		results = append(results, result{i, atSec, match, misalg})
	}

	fmt.Printf("trickplay sidecar has %d frames; sampling %d in middle 80%%\n\n", total, len(results))
	fmt.Printf("%-5s %-10s %-12s %-15s\n", "idx", "t(s)", "match Hamming", "next-tile (control)")
	fmt.Println("-----------------------------------------------------")
	matched := 0
	for _, r := range results {
		flag := "  "
		if r.hammingMatch <= 12 {
			flag = "✓"
			matched++
		} else if r.hammingMatch <= 20 {
			flag = "~"
		} else {
			flag = "✗"
		}
		fmt.Printf("%s %-3d %-9.3f %-12d %-3d\n", flag, r.idx, r.atSec, r.hammingMatch, r.hammingMisalg)
	}
	fmt.Printf("\n%d/%d tiles match their declared timestamp (Hamming ≤12)\n", matched, len(results))
}
