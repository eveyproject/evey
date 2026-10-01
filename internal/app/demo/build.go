package demo

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evey-project/evey/internal/pkg/demo"
)

// Assembler runs ffmpeg with args (everything after the binary name). The
// composition root binds it to a forked ffmpeg; tests to a fake.
type Assembler func(ctx context.Context, args []string) error

// WithAssembler enables the manifest's Build entries. Without it they are
// skipped, logged, and the rest of the demo downloads as usual.
func (b *Bootstrapper) WithAssembler(a Assembler) *Bootstrapper {
	b.assemble = a
	return b
}

// build downloads every input of f.Build next to dest, muxes them into dest
// through a .part file, and removes the inputs. An interrupted build leaves
// no media file behind and starts over at the next boot.
func (b *Bootstrapper) build(ctx context.Context, f demo.MediaFile, dest string) error {
	if b.assemble == nil {
		return errors.New("no assembler wired")
	}
	work := dest + ".build"
	if err := os.MkdirAll(work, 0o755); err != nil {
		return err
	}
	defer func() {
		if err := os.RemoveAll(work); err != nil {
			b.log.Warn("demo: remove build folder", "path", work, "err", err)
		}
	}()
	args := []string{"-hide_banner", "-nostdin", "-y", "-loglevel", "error"}
	for i, in := range f.Build.Inputs {
		p := filepath.Join(work, fmt.Sprintf("in%02d%s", i, in.Ext))
		if err := b.fetch(ctx, demo.MediaFile{URL: in.URL, ZipMember: in.ZipMember}, p); err != nil {
			return fmt.Errorf("input %d: %w", i, err)
		}
		args = append(args, "-i", p)
	}
	if len(f.Build.Chapters) > 0 {
		meta := filepath.Join(work, "chapters.txt")
		if err := os.WriteFile(meta, []byte(ffmetadata(f.Build.Chapters, f.Build.DurationSec)), 0o644); err != nil {
			return err
		}
		args = append(args, "-i", meta, "-map_chapters", fmt.Sprint(len(f.Build.Inputs)))
	}
	part := dest + ".part"
	args = append(args, f.Build.Args...)
	args = append(args, "-f", muxerFor(dest), part)
	if err := b.assemble(ctx, args); err != nil {
		b.removeQuietly(part)
		return fmt.Errorf("mux %s: %w", f.Path, err)
	}
	if st, err := os.Stat(part); err != nil || st.Size() == 0 {
		b.removeQuietly(part)
		return fmt.Errorf("mux %s: empty output", f.Path)
	}
	return os.Rename(part, dest)
}

// ffmetadata writes the chapter marks in ffmpeg's metadata format, each mark
// ending where the next starts and the last at the end of the film.
func ffmetadata(chapters []demo.Chapter, durationSec int) string {
	var sb strings.Builder
	sb.WriteString(";FFMETADATA1\n")
	for i, c := range chapters {
		end := int64(durationSec) * 1000
		if i+1 < len(chapters) {
			end = int64(chapters[i+1].StartSec) * 1000
		}
		fmt.Fprintf(&sb, "[CHAPTER]\nTIMEBASE=1/1000\nSTART=%d\nEND=%d\ntitle=%s\n", int64(c.StartSec)*1000, end, escapeMeta(c.Title))
	}
	return sb.String()
}

func escapeMeta(s string) string {
	r := strings.NewReplacer(`\`, `\\`, "=", `\=`, ";", `\;`, "#", `\#`, "\n", `\`+"\n")
	return r.Replace(s)
}

// muxerFor names the output format: the .part suffix hides the extension
// ffmpeg would otherwise pick it from.
func muxerFor(dest string) string {
	switch strings.ToLower(filepath.Ext(dest)) {
	case ".mkv":
		return "matroska"
	case ".webm":
		return "webm"
	default:
		return "mp4"
	}
}

// removeRetired deletes files an earlier demo downloaded that a richer one
// replaced, reporting the library folders it touched for a rescan.
func (b *Bootstrapper) removeRetired(touched map[string]bool) {
	for _, rel := range demo.RetiredMedia {
		p := filepath.Join(b.mediaDir, filepath.FromSlash(rel))
		if err := os.Remove(p); err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				b.log.Warn("demo: remove retired media", "file", rel, "err", err)
			}
			continue
		}
		b.log.Info("demo: retired media removed", "file", rel)
		touched[topFolder(rel)] = true
	}
}
