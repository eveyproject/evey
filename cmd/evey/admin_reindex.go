package main

import (
	"context"
	"flag"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// adminReindexNoCue walks a directory tree and rebuilds the seek index (Cues)
// of every EBML file (MKV/WebM) that lacks one — the fix for slow resume on
// files muxed without an index, where the player has to linearly scan to seek.
//
// It is filesystem-only (no DB): point it at a library root, review the
// dry-run, then re-run with --apply.
//
// Safety (locked by design): dry-run by default; each file is reindexed to a
// temp, then fully re-parsed + validated (header, tracks AND every cluster
// block) and cross-checked against the original (track count, matching
// duration, a freshly-present index) before an atomic swap. The original is
// kept as "<file>.mkvgobak" unless --keep-backup=false. A file that fails any
// check is left byte-for-byte untouched.
func adminReindexNoCue(args []string) error {
	flags := flag.NewFlagSet("reindex-no-cue", flag.ExitOnError)
	root := flags.String("root", "", "directory tree to scan (required)")
	apply := flags.Bool("apply", false, "actually rewrite files (default: dry-run, report only)")
	keepBackup := flags.Bool("keep-backup", true, "keep the .mkvgobak copy of each rewritten original")
	limit := flags.Int("limit", 0, "stop after rewriting N files (0 = no limit; for a cautious first run)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *root == "" {
		flags.Usage()
		return fmt.Errorf("--root is required")
	}
	info, err := os.Stat(*root)
	if err != nil {
		return fmt.Errorf("stat root: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("--root must be a directory (got %s)", *root)
	}

	adapter := mkvgoadapter.New()
	ctx := context.Background()

	mode := "DRY-RUN (no files modified — re-run with --apply to rewrite)"
	if *apply {
		mode = "APPLY (files rewritten; originals kept as .mkvgobak)"
		if !*keepBackup {
			mode = "APPLY (files rewritten; NO backup kept)"
		}
	}
	fmt.Printf("reindex-no-cue\n  root: %s\n  mode: %s\n\n", *root, mode)

	var scanned, alreadyIndexed, noIndex, reindexed, failed int
	stop := false
	walkErr := filepath.WalkDir(*root, func(path string, d fs.DirEntry, walkErr error) error {
		if stop {
			return filepath.SkipAll
		}
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
			failed++
			return nil
		}
		if has {
			alreadyIndexed++
			return nil
		}
		noIndex++
		if !*apply {
			fmt.Printf("  would reindex: %s\n", path)
			return nil
		}
		res, err := adapter.ReindexInPlace(ctx, path, mkv.ReindexOptions{KeepBackup: *keepBackup})
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! reindex %s: %v (original untouched)\n", path, err)
			failed++
			return nil
		}
		if res.Skipped { // gained an index between the scan and the rewrite
			alreadyIndexed++
			noIndex--
			return nil
		}
		reindexed++
		bak := res.BackupPath
		if bak == "" {
			bak = "(removed)"
		}
		fmt.Printf("  reindexed: %s  (%d -> %d bytes, backup: %s)\n", path, res.SrcBytes, res.DstBytes, bak)
		for _, w := range res.Warnings {
			fmt.Printf("      validation warning: %s\n", w)
		}
		if *limit > 0 && reindexed >= *limit {
			fmt.Printf("  (--limit %d reached, stopping)\n", *limit)
			stop = true
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk: %w", walkErr)
	}

	fmt.Printf("\nsummary: %d EBML files scanned, %d already indexed, %d without index", scanned, alreadyIndexed, noIndex)
	if *apply {
		fmt.Printf(", %d reindexed, %d failed (originals safe)\n", reindexed, failed)
		fmt.Printf("note: rewritten files changed on disk — re-scan the library so it re-probes them.\n")
	} else {
		fmt.Printf(" (dry-run — nothing modified)\n")
	}
	return nil
}
