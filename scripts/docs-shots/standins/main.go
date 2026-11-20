// Command standins fills a demo media directory for the documentation
// screenshots without the full download.
//
// The demo manifest weighs gigabytes, yet the screenshots only ever show the
// pictures of a few films: everything else appears as a TMDB poster, a title,
// a duration or a resolution badge. Every other video of the manifest is
// written here as a stand-in: a black picture with silence, the duration and
// the frame size of the real file, in the same container. The demo bootstrap
// keeps a file that is already there, so it downloads only what is kept.
//
// Usage: standins -dir <demo media dir> [-ffmpeg ffmpeg] [-ffprobe ffprobe]
package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/eveyproject/evey/internal/pkg/demo"
)

// kept are the files whose own pictures a screenshot shows (the player, the
// now playing cards, the stream info panel): the bootstrap downloads them for
// real.
var kept = map[string]bool{
	"movies/Spring (2019).webm":       true,
	"movies/Sprite Fright (2021).mkv": true,
}

// shape is what a stand-in copies from the real file.
type shape struct {
	width, height int
	seconds       float64
}

// zipped are the files served inside an archive: ffprobe cannot reach the
// member over HTTP, so their shape is written down.
var zipped = map[string]shape{
	"movies/Big Buck Bunny (2008).mp4": {3840, 2160, 634.5},
}

var videoExt = map[string]bool{".mp4": true, ".mkv": true, ".webm": true, ".ogv": true, ".mov": true}

func main() {
	dir := flag.String("dir", "", "demo media directory (EVEY_DEMO_MEDIA_DIR)")
	ffmpeg := flag.String("ffmpeg", "ffmpeg", "ffmpeg binary")
	ffprobe := flag.String("ffprobe", "ffprobe", "ffprobe binary")
	flag.Parse()
	if *dir == "" {
		fmt.Fprintln(os.Stderr, "standins: -dir is required")
		os.Exit(2)
	}
	ctx := context.Background()
	made, failed := 0, 0
	for _, f := range demo.Manifest {
		if kept[f.Path] || !videoExt[strings.ToLower(filepath.Ext(f.Path))] {
			continue
		}
		dest := filepath.Join(*dir, f.Path)
		if st, err := os.Stat(dest); err == nil && st.Size() > 0 {
			continue
		}
		s, err := shapeOf(ctx, *ffprobe, f)
		if err != nil {
			fmt.Fprintf(os.Stderr, "standins: %s: %v\n", f.Path, err)
			failed++
			continue
		}
		if err := write(ctx, *ffmpeg, dest, s); err != nil {
			fmt.Fprintf(os.Stderr, "standins: %s: %v\n", f.Path, err)
			failed++
			continue
		}
		fmt.Printf("stand-in %s (%dx%d, %.0f s)\n", f.Path, s.width, s.height, s.seconds)
		made++
	}
	fmt.Printf("standins: %d written, %d failed\n", made, failed)
	if failed > 0 {
		os.Exit(1)
	}
}

// shapeOf reads the real file's frame size and duration. ffprobe reads only
// the headers over HTTP (range requests), not the file.
func shapeOf(ctx context.Context, ffprobe string, f demo.MediaFile) (shape, error) {
	if s, ok := zipped[f.Path]; ok {
		return s, nil
	}
	url := f.URL
	var seconds float64
	if f.Build != nil {
		url = f.Build.Inputs[0].URL
		seconds = float64(f.Build.DurationSec)
	}
	out, err := probe(ctx, ffprobe, url)
	if err != nil {
		return shape{}, err
	}
	var probe struct {
		Streams []struct{ Width, Height int } `json:"streams"`
		Format  struct{ Duration string }     `json:"format"`
	}
	if err := json.Unmarshal(out, &probe); err != nil {
		return shape{}, fmt.Errorf("ffprobe output: %w", err)
	}
	if len(probe.Streams) == 0 || probe.Streams[0].Width == 0 {
		return shape{}, fmt.Errorf("ffprobe: no video stream")
	}
	if seconds == 0 {
		seconds, _ = strconv.ParseFloat(probe.Format.Duration, 64)
	}
	if seconds <= 0 {
		return shape{}, fmt.Errorf("ffprobe: no duration")
	}
	return shape{probe.Streams[0].Width, probe.Streams[0].Height, seconds}, nil
}

// userAgent is the one the demo bootstrap downloads with: Wikimedia answers
// 429 to a client that does not say who it is.
const userAgent = "Evey-demo-bootstrap/1.0 (+https://eveyproject.org)"

// probe asks ffprobe for the first video stream's size and the duration,
// again after a pause when the host is busy.
func probe(ctx context.Context, ffprobe, url string) ([]byte, error) {
	var last error
	for attempt := range 4 {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 20 * time.Second)
		}
		pctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		var stderr strings.Builder
		cmd := exec.CommandContext(pctx, ffprobe, "-v", "error", "-user_agent", userAgent, "-select_streams", "v:0",
			"-show_entries", "stream=width,height:format=duration", "-of", "json", url)
		cmd.Stderr = &stderr
		out, err := cmd.Output()
		cancel()
		if err == nil {
			return out, nil
		}
		last = fmt.Errorf("ffprobe: %w: %s", err, strings.TrimSpace(stderr.String()))
	}
	return nil, last
}

// write encodes a black, silent file of that shape, one frame a second and a
// keyframe every ten, in the container the extension names. The keyframes
// matter: with the encoder's default of one every 250 frames, Media health
// and the keyframe index see a file no real film looks like.
func write(ctx context.Context, ffmpeg, dest string, s shape) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	var codecs []string
	switch strings.ToLower(filepath.Ext(dest)) {
	case ".webm":
		codecs = []string{"-c:v", "libvpx", "-deadline", "realtime", "-cpu-used", "8", "-b:v", "50k", "-c:a", "libopus", "-b:a", "16k"}
	case ".ogv":
		codecs = []string{"-c:v", "libtheora", "-q:v", "0", "-c:a", "libvorbis", "-q:a", "0"}
	default:
		codecs = []string{"-c:v", "libx264", "-preset", "ultrafast", "-tune", "stillimage", "-pix_fmt", "yuv420p", "-c:a", "aac", "-b:a", "32k"}
	}
	part := dest + ".standin" + filepath.Ext(dest)
	args := []string{"-v", "error", "-y",
		"-f", "lavfi", "-i", fmt.Sprintf("color=c=black:s=%dx%d:r=1", s.width, s.height),
		"-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo",
		"-t", strconv.FormatFloat(s.seconds, 'f', 3, 64), "-g", "10"}
	args = append(append(args, codecs...), part)
	if out, err := exec.CommandContext(ctx, ffmpeg, args...).CombinedOutput(); err != nil {
		err = fmt.Errorf("ffmpeg: %w: %s", err, strings.TrimSpace(string(out)))
		if rmErr := os.Remove(part); rmErr != nil && !errors.Is(rmErr, fs.ErrNotExist) {
			err = errors.Join(err, rmErr)
		}
		return err
	}
	return os.Rename(part, dest)
}
