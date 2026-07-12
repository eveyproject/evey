package main

import (
	"context"
	"flag"
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
	flags := flag.NewFlagSet("reindex-no-cue", flag.ExitOnError)
	root := flags.String("root", "", "directory tree to scan (required)")
	apply := flags.Bool("apply", false, "actually rewrite files (default: dry-run, report only)")
	keepBackup := flags.Bool("keep-backup", true, "keep the .mkvgobak copy of each rewritten original (rewrites only; ignored by --mode inplace)")
	limit := flags.Int("limit", 0, "stop after rewriting N files (0 = no limit; for a cautious first run)")
	modeFlag := flags.String("mode", "auto", "repair strategy: auto (surgical patch when the layout allows, rewrite otherwise), inplace (patch ONLY, no backup, refuses defeated layouts), rewrite (always rebuild + swap)")
	deepVerify := flags.Bool("deep-verify", false, "prove every rebuilt cue lands on a real video keyframe before committing (full read per file); forced on when --keep-backup=false")
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

	// Phase 1: discover the un-indexed candidates. HasSeekIndex is a head-only
	// probe, so this walk is fast even on a large, network-mounted tree.
	var scanned, alreadyIndexed int
	var candidates []string
	walkErr := filepath.WalkDir(*root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "  ! walk %s: %v\n", path, walkErr)
			return nil // keep scanning the rest of the tree
		}
		if d.IsDir() || !mkvgoadapter.IsEBMLContainer(path) {
			return nil
		}
		scanned++
		has, err := adapter.HasSeekIndex(ctx, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! read %s: %v\n", path, err)
			return nil
		}
		if has {
			alreadyIndexed++
			return nil
		}
		candidates = append(candidates, path)
		if !*apply {
			fmt.Printf("  would reindex: %s\n", path)
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk: %w", walkErr)
	}

	if !*apply {
		fmt.Printf("\nsummary: %d EBML files scanned, %d already indexed, %d without index (dry-run - nothing modified)\n",
			scanned, alreadyIndexed, len(candidates))
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

	var reindexed, failed, skipped int
	for i, path := range candidates {
		res, err := adapter.ReindexInPlace(ctx, path, mkv.ReindexOptions{
			Mode:       reindexMode,
			KeepBackup: *keepBackup,
			DeepVerify: verify,
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
		}
		done := i + 1
		if done%reindexProgressBatch == 0 || done == total {
			renderReindexProgress(done, total, reindexed, failed, filepath.Base(path))
		}
	}
	fmt.Println() // close the progress line

	fmt.Printf("\nsummary: %d EBML files scanned, %d already indexed, %d reindexed, %d failed, %d skipped (originals safe)\n",
		scanned, alreadyIndexed, reindexed, failed, skipped)
	fmt.Printf("note: rewritten files changed on disk - re-scan the library so it re-probes them.\n")
	return nil
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
