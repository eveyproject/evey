package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// adminReindexNoCue walks a directory tree and rebuilds the seek index (Cues)
// of every EBML file (MKV/WebM) that lacks one - the fix for slow resume on
// files muxed without an index, where the player has to linearly scan to seek.
//
// It is filesystem-only (no DB): point it at a library root, review the
// dry-run, then re-run with --apply.
//
// Safety (locked by design): dry-run by default. --mode auto (default) takes
// the surgical in-place patch when the file layout allows it (journaled,
// rolled back on any failed check) and otherwise rewrites to a temp that is
// fully re-parsed + validated (header, tracks AND every cluster block) and
// cross-checked against the original (track count, matching duration, a
// freshly-present index) before an atomic swap. A rewrite keeps the original
// as "<file>.mkvgobak" unless --keep-backup=false, which enforces
// --deep-verify instead. A file that fails any check is left byte-for-byte
// untouched. The rewrite also refuses upfront when the volume lacks the room
// for its temporary copy.
func adminReindexNoCue(args []string) error {
	flags := adminFlags("reindex-no-cue")
	root := flags.String("root", "", "directory tree to scan (required)")
	apply := flags.Bool("apply", false, "actually rewrite files (default: dry-run, report only)")
	keepBackup := flags.Bool("keep-backup", true, "keep the .mkvgobak copy of each rewritten original (rewrites only; ignored by --mode inplace)")
	limit := flags.Int("limit", 0, "stop after rewriting N files (0 = no limit; for a cautious first run)")
	modeFlag := flags.String("mode", "auto", "repair strategy: auto (surgical patch when the layout allows, rewrite otherwise), inplace (patch ONLY, no backup, refuses defeated layouts), rewrite (always rebuild + swap)")
	deepVerify := flags.Bool("deep-verify", false, "prove every rebuilt cue lands on a real video keyframe before committing (full read per file); forced on when --keep-backup=false")
	resync := flags.Bool("resync", false, "repair files with a CORRUPTED region (junk between clusters, a lying size) that the strict reindex refuses: skip the damaged span and resynchronise, recovering everything before and after. Always rewrites + keeps a backup (this loses the skipped bytes); the lost spans are reported")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		flags.Usage()
		return fmt.Errorf("--root is required")
	}
	var reindexMode string
	switch *modeFlag {
	case "auto":
		reindexMode = mkv.ModeAuto
	case mkv.ModeInPlace, mkv.ModeRewrite:
		reindexMode = *modeFlag
	default:
		return fmt.Errorf("--mode must be auto, inplace or rewrite (got %q)", *modeFlag)
	}
	// Without a rollback copy, the full-read proof is the safety net.
	verify := *deepVerify || (!*keepBackup && reindexMode != mkv.ModeInPlace)
	info, err := os.Stat(*root)
	if err != nil {
		return fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--root must be a directory (got %s)", *root)
	}

	adapter := mkvgoadapter.New()
	ctx := context.Background()

	runLabel := "DRY-RUN (no files modified - re-run with --apply to rewrite)"
	if *apply {
		switch {
		case reindexMode == mkv.ModeInPlace:
			runLabel = "APPLY (surgical in-place patch only; no duplicate, no backup)"
		case *keepBackup:
			runLabel = "APPLY (files repaired; rewrite originals kept as .mkvgobak)"
		default:
			runLabel = "APPLY (files repaired; NO backup kept, deep-verify enforced)"
		}
	}
	fmt.Printf("reindex-no-cue\n  root: %s\n  strategy: %s\n  mode: %s\n\n", *root, *modeFlag, runLabel)

	// Phase 1: diagnose every EBML file. mkvgo's Diagnose is one head-mostly
	// read (it walks only when the declared size diverges), so it stays fast on
	// a large network-mounted tree while telling apart three cases HasSeekIndex
	// could not: a present-but-unusable index (audio-keyed/stale cues) that
	// still needs a reindex, mid-file damage a resync can skip past, and a
	// truncated download whose missing tail NO repair recovers - only
	// re-acquiring the source. Truncated files are reported apart and left out
	// of the repair set unless --resync is asked (then a resync recovers the
	// playable prefix, with a per-file warning it is not the whole film).
	var scanned, alreadyIndexed int
	var candidates, truncated []string
	walkErr := filepath.WalkDir(*root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "  ! walk %s: %v\n", path, walkErr)
			return nil // keep scanning the rest of the tree
		}
		if d.IsDir() || !mkvgoadapter.IsEBMLContainer(path) {
			return nil
		}
		scanned++
		diag, err := adapter.Diagnose(ctx, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! read %s: %v\n", path, err)
			return nil
		}
		if diag.Truncated {
			truncated = append(truncated, path)
			if *resync {
				candidates = append(candidates, path) // recover the prefix, with a warning below
			}
			if !*apply {
				fmt.Printf("  incomplete download (re-acquire the source%s): %s\n",
					map[bool]string{true: "; --resync recovers only the playable prefix", false: ""}[*resync], path)
			}
			return nil
		}
		if !diag.HasIndexDefect() && diag.DamagedBytes == 0 {
			alreadyIndexed++
			return nil
		}
		candidates = append(candidates, path)
		if !*apply {
			if diag.DamagedBytes > 0 {
				fmt.Printf("  would repair (mid-file damage, ~%d bytes): %s\n", diag.DamagedBytes, path)
			} else {
				fmt.Printf("  would reindex: %s\n", path)
			}
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk: %w", walkErr)
	}

	if len(truncated) > 0 {
		fmt.Printf("\n%d incomplete download(s) found - these cannot be repaired, re-acquire the source:\n", len(truncated))
		for _, p := range truncated {
			fmt.Printf("  - %s\n", p)
		}
		fmt.Println()
	}

	if !*apply {
		fmt.Printf("\nsummary: %d EBML files scanned, %d already indexed, %d to repair, %d truncated (re-acquire) (dry-run - nothing modified)\n",
			scanned, alreadyIndexed, len(candidates), len(truncated))
		return nil
	}

	// --limit caps a cautious first run to the first N files.
	if *limit > 0 && len(candidates) > *limit {
		fmt.Printf("  (--limit %d: repairing the first %d of %d candidates)\n", *limit, *limit, len(candidates))
		candidates = candidates[:*limit]
	}

	// Phase 2: repair, with a progress bar. This is the long part: every file
	// is rewritten in full, so a whole library can take hours. The progress
	// bar refreshes every reindexProgressBatch files.
	total := len(candidates)
	if total == 0 {
		fmt.Printf("\nsummary: %d EBML files scanned, %d already indexed, 0 without index - nothing to do\n", scanned, alreadyIndexed)
		return nil
	}
	fmt.Printf("repairing %d file(s) - this is a LONG operation: each file is rewritten in full and a large library can take hours.\n", total)
	fmt.Printf("originals are kept as .mkvgobak%s. Progress updates every %d files:\n\n",
		map[bool]string{true: "", false: " until each swap, then removed"}[*keepBackup], reindexProgressBatch)

	truncatedSet := make(map[string]bool, len(truncated))
	for _, p := range truncated {
		truncatedSet[p] = true
	}
	var reindexed, failed, skipped int
	var lostBytes int64
	for i, path := range candidates {
		if truncatedSet[path] {
			fmt.Fprintf(os.Stderr, "\r  ! %s is an incomplete download: resync recovers the playable prefix only, not the missing tail (re-acquire for the whole film)\n",
				filepath.Base(path))
		}
		res, err := adapter.ReindexInPlace(ctx, path, mkv.ReindexOptions{
			Mode:       reindexMode,
			KeepBackup: *keepBackup,
			DeepVerify: verify,
			Resync:     *resync, // repair corrupted regions instead of refusing them
		})
		switch {
		case err != nil:
			failed++
			fmt.Fprintf(os.Stderr, "\r  ! reindex %s: %v (original untouched)\n", filepath.Base(path), err)
		case res.Skipped: // gained an index between the scan and the rewrite
			skipped++
		default:
			reindexed++
			for _, w := range res.Warnings {
				fmt.Fprintf(os.Stderr, "\r      validation warning (%s): %s\n", filepath.Base(path), w)
			}
			for _, r := range res.DamagedRanges {
				n := r.EndOffset - r.StartOffset
				lostBytes += n
				fmt.Fprintf(os.Stderr, "\r      resync skipped a corrupted region (%s): %d bytes lost around %s\n",
					filepath.Base(path), n, formatApproxMs(r.ApproxStartMs))
			}
		}
		done := i + 1
		if done%reindexProgressBatch == 0 || done == total {
			renderReindexProgress(done, total, reindexed, failed, filepath.Base(path))
		}
	}
	fmt.Println() // close the progress line

	fmt.Printf("\nsummary: %d EBML files scanned, %d already indexed, %d reindexed, %d failed, %d skipped (originals safe)\n",
		scanned, alreadyIndexed, reindexed, failed, skipped)
	if len(truncated) > 0 {
		fmt.Printf("%d truncated file(s) are incomplete downloads - re-acquire the source (a resync only rebuilds a playable prefix).\n", len(truncated))
	}
	if *resync && lostBytes > 0 {
		fmt.Printf("resync recovered the files around %d corrupted byte(s) total (the rest is intact; backups kept).\n", lostBytes)
	}
	fmt.Printf("note: rewritten files changed on disk - re-scan the library so it re-probes them.\n")
	return nil
}

// formatApproxMs renders a presentation time as HH:MM:SS for the resync report.
func formatApproxMs(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, (s%3600)/60, s%60)
}

// reindexProgressBatch is how many files are processed between progress-bar
// refreshes - the "batch of 5" cadence.
const reindexProgressBatch = 5

// renderReindexProgress overwrites a single terminal line with a bar, the
// done/total count and the current file. Carriage-return only (no newline) so
// it updates in place; failures/warnings print above it with their own \r + \n.
func renderReindexProgress(done, total, reindexed, failed int, current string) {
	const width = 30
	filled := min(done*width/total, width)
	bar := strings.Repeat("=", filled) + strings.Repeat(" ", width-filled)
	pct := done * 100 / total
	name := current
	if len(name) > 40 {
		name = name[:37] + "..."
	}
	fmt.Printf("\r  [%s] %3d%%  %d/%d  (%d ok, %d failed)  %-40s", bar, pct, done, total, reindexed, failed, name)
}
