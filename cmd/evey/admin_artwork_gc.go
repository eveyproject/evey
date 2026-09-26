package main

import (
	"context"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// adminArtworkGC deletes artwork files on disk that no DB row references
// (artwork.local_path / artwork.webp_path). Such orphans piled up under the
// old scheme where the on-disk filename was the ArtworkRecord UUID: every
// re-enrichment minted a fresh record (new UUID → new file) and the old file
// leaked because DeleteByMediaItemID drops DB rows, not files - observed as
// hundreds of GB of byte-identical copies. Content-addressing (keying the file
// on the source URL) stops new orphans; this reclaims the existing pile.
//
// Dry-run by default: prints what it WOULD delete. Pass --apply to delete.
func adminArtworkGC(args []string) error {
	fset := adminFlags("artwork-gc")
	root := fset.String("root", "/data/artworks", "artwork directory to scan (server default: /data/artworks)")
	apply := fset.Bool("apply", false, "actually delete orphans (default: dry-run)")
	if err := fset.Parse(args); err != nil {
		return err
	}

	if fi, err := os.Stat(*root); err != nil || !fi.IsDir() {
		return fmt.Errorf("artwork root %q not a readable directory: %w", *root, err)
	}

	_, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	// Every path any artwork row points at - both the original and the webp
	// variant. A file under root not in this set is an orphan.
	referenced := make(map[string]struct{})
	rows, err := db.QueryContext(ctx,
		`SELECT local_path, webp_path FROM artwork WHERE local_path IS NOT NULL OR webp_path IS NOT NULL`)
	if err != nil {
		return fmt.Errorf("query artwork paths: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var lp, wp *string
		if scanErr := rows.Scan(&lp, &wp); scanErr != nil {
			return fmt.Errorf("scan: %w", scanErr)
		}
		if lp != nil && *lp != "" {
			referenced[filepath.Clean(*lp)] = struct{}{}
		}
		if wp != nil && *wp != "" {
			referenced[filepath.Clean(*wp)] = struct{}{}
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("rows: %w", err)
	}

	// TOCTOU guard: a file written + DB-updated AFTER the referenced-set snapshot
	// (a concurrent enrichment downloading new artwork) is not yet in the set, so
	// it would look orphaned. Skip anything modified within this window so an
	// in-flight download is never deleted out from under a fresh DB row.
	recentCutoff := time.Now().Add(-5 * time.Minute)
	var scanned, orphans, refFound, inflight int
	var orphanBytes int64
	var toDelete []string
	walkErr := filepath.WalkDir(*root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // unreadable entry: skip it, keep walking the rest
		}
		if d.IsDir() {
			return nil
		}
		scanned++
		if _, ok := referenced[filepath.Clean(path)]; ok {
			refFound++
			return nil
		}
		info, statErr := d.Info()
		if statErr == nil && info.ModTime().After(recentCutoff) {
			inflight++ // recently written: a concurrent download not yet in the snapshot
			return nil
		}
		if statErr == nil {
			orphanBytes += info.Size()
		}
		orphans++
		toDelete = append(toDelete, path)
		return nil
	})
	if walkErr != nil {
		return fmt.Errorf("walk %s: %w", *root, walkErr)
	}

	fmt.Printf("artwork-gc: scanned=%d files, db-references=%d, matched-under-root=%d, in-flight-skipped=%d, orphans=%d (%.2f GB)\n",
		scanned, len(referenced), refFound, inflight, orphans, float64(orphanBytes)/(1<<30))

	// Safety: if the DB references artwork but NONE of those files were found
	// under root, the paths don't line up (wrong --root, or DB paths use a
	// different prefix than this host) and every file looks orphaned. Refuse to
	// delete - otherwise a prefix mismatch wipes the whole cache.
	if *apply && len(referenced) > 0 && refFound == 0 && scanned > 0 {
		return fmt.Errorf("refusing --apply: 0 of %d DB-referenced files found under %s - likely a wrong --root / path-prefix mismatch", len(referenced), *root)
	}

	if !*apply {
		fmt.Println("dry-run - pass --apply to delete. Sample:")
		for i, p := range toDelete {
			if i >= 10 {
				fmt.Printf("  ... and %d more\n", orphans-10)
				break
			}
			fmt.Printf("  would delete %s\n", p)
		}
		return nil
	}

	var deleted int
	var freed int64
	for _, p := range toDelete {
		info, statErr := os.Stat(p)
		if rmErr := os.Remove(p); rmErr != nil {
			fmt.Fprintf(os.Stderr, "  remove %s: %v\n", p, rmErr)
			continue
		}
		deleted++
		if statErr == nil {
			freed += info.Size()
		}
	}
	// Remove the per-media subdirectories left empty after deleting their files
	// (the walk only removed files). Best-effort: a dir that isn't empty (still
	// has a referenced file) or races a concurrent write just stays.
	var removedDirs int
	if entries, readErr := os.ReadDir(*root); readErr == nil {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			dir := filepath.Join(*root, e.Name())
			if sub, _ := os.ReadDir(dir); len(sub) == 0 {
				if os.Remove(dir) == nil {
					removedDirs++
				}
			}
		}
	}

	fmt.Printf("artwork-gc: deleted %d files, reclaimed %.2f GB, removed %d empty dirs\n", deleted, float64(freed)/(1<<30), removedDirs)
	fmt.Fprintf(os.Stderr, "[admin] artwork-gc deleted %d orphan files (%.2f GB) under %s\n", deleted, float64(freed)/(1<<30), *root)
	return nil
}
