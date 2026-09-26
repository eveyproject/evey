package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	mkvgoadapter "github.com/evey-project/evey/internal/adapters/driven/mkvgo"
	"github.com/evey-project/evey/internal/domain/mkv"
)

// adminValidateStreaming walks a directory tree and reports each EBML file's
// streaming-readiness (mkvgo's ops.Validate): a missing Cues index, cue points
// on a non-video track, cue times matching no keyframe, video tracks without a
// DefaultDuration, subtitle blocks without a BlockDuration, AAC without its
// AudioSpecificConfig. These are exactly what the on-demand HLS copy rung
// (mp4.PlanHLS) and fast seeking rely on - a file with errors falls back to the
// ffmpeg pipeline instead of stateless packaging. Read-only: it never touches a
// file; `reindex-no-cue` is the fix for a missing index.
func adminValidateStreaming(args []string) error {
	flags := adminFlags("validate-streaming")
	root := flags.String("root", "", "directory tree to scan (required)")
	errorsOnly := flags.Bool("errors-only", false, "report only error-severity findings (skip warnings)")
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

	fmt.Printf("validate-streaming\n  root: %s\n\n", *root)

	var scanned, clean, withErrors, withWarnings, failed int
	walkErr := filepath.WalkDir(*root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			fmt.Fprintf(os.Stderr, "  ! walk %s: %v\n", path, walkErr)
			return nil
		}
		if d.IsDir() || !mkvgoadapter.IsEBMLContainer(path) {
			return nil
		}
		scanned++
		issues, err := adapter.ValidateStreaming(ctx, path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "  ! validate %s: %v\n", path, err)
			failed++
			return nil
		}
		var errs, warns []mkv.ValidationIssue
		for _, iss := range issues {
			if iss.Severity == mkv.ValidationError {
				errs = append(errs, iss)
			} else {
				warns = append(warns, iss)
			}
		}
		switch {
		case len(errs) > 0:
			withErrors++
		case len(warns) > 0:
			withWarnings++
		default:
			clean++
			return nil
		}
		if *errorsOnly && len(errs) == 0 {
			return nil
		}
		fmt.Printf("  %s\n", path)
		for _, iss := range errs {
			fmt.Printf("      ERROR:   %s\n", iss.Message)
		}
		if !*errorsOnly {
			for _, iss := range warns {
				fmt.Printf("      warning: %s\n", iss.Message)
			}
		}
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk: %w", walkErr)
	}

	fmt.Printf("\nsummary: %d EBML files scanned, %d streaming-ready, %d with errors, %d with warnings only, %d unreadable\n",
		scanned, clean, withErrors, withWarnings, failed)
	if withErrors > 0 {
		fmt.Printf("note: files with errors seek poorly and skip the stateless on-demand rung; run `reindex-no-cue --apply` to rebuild a missing index.\n")
	}
	return nil
}
