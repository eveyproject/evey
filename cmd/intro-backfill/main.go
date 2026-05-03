// intro-backfill triggers intro detection on a subset of media items in the
// catalog. Use after a behavior change in the detection pipeline to refresh
// stale segments without waiting for the worker auto-trigger to fire on
// trickplay regeneration.
//
// Reads media_item IDs from Postgres (no need to know the API), then POSTs
// to /api/v1/admin/media/{id}/detect-segments with a rate limit that
// respects the server's concurrency cap (defaultMaxParallelDetections=2).
//
// Usage:
//
//	intro-backfill \
//	  --db postgres://evey:evey@localhost:5432/evey \
//	  --api http://localhost:9091 \
//	  --token "$(curl -s -X POST .../login | jq -r .accessToken)" \
//	  [--filter "series='9-1-1'"] \
//	  [--only-without-chapter-source] \
//	  [--limit 100] \
//	  [--max-parallel 2] \
//	  [--dry-run]
package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	_ "github.com/lib/pq"
)

func main() {
	var (
		dbURL                   = flag.String("db", envOr("EVEY_DB_URL", "postgres://evey:evey@localhost:5432/evey?sslmode=disable"), "Postgres connection string")
		apiBase                 = flag.String("api", envOr("EVEY_API", "http://localhost:9091"), "API base URL")
		token                   = flag.String("token", os.Getenv("EVEY_TOKEN"), "admin JWT (or EVEY_TOKEN env)")
		filter                  = flag.String("filter", "", "extra SQL WHERE clause, e.g. \"series='9-1-1'\" — joined to media_items via parent series")
		onlyWithoutChapterSource = flag.Bool("only-without-chapter-source", false, "limit to media items whose existing intro segment is NOT mkv_chapter (typical use after chapter pass goes live)")
		onlyWithChapters        = flag.Bool("only-with-chapters", false, "limit to media items that have chapters in DB (a no-op when chapter pass is the only intent)")
		limit                   = flag.Int("limit", 0, "max number of items to trigger (0 = all)")
		maxParallel             = flag.Int("max-parallel", 2, "concurrent triggers (must match server's intro detection concurrency cap)")
		dryRun                  = flag.Bool("dry-run", false, "list matching items without triggering")
	)
	flag.Parse()

	if *token == "" && !*dryRun {
		fmt.Fprintln(os.Stderr, "error: --token (or EVEY_TOKEN env) is required unless --dry-run")
		os.Exit(2)
	}

	db, err := sql.Open("postgres", *dbURL)
	if err != nil {
		fmt.Fprintf(os.Stderr, "open db: %v\n", err)
		os.Exit(1)
	}
	defer db.Close()

	ids, titles, err := loadMediaIDs(context.Background(), db, *filter, *onlyWithoutChapterSource, *onlyWithChapters, *limit)
	if err != nil {
		fmt.Fprintf(os.Stderr, "load ids: %v\n", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		fmt.Println("nothing to backfill")
		return
	}

	fmt.Printf("matched %d media items\n", len(ids))
	if *dryRun {
		for i, id := range ids {
			fmt.Printf("  %3d. %s  %s\n", i+1, id, titles[i])
		}
		return
	}

	var (
		ok    atomic.Int64
		fail  atomic.Int64
		sem   = make(chan struct{}, *maxParallel)
		wg    sync.WaitGroup
		start = time.Now()
	)
	for i, id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int, id, title string) {
			defer wg.Done()
			defer func() { <-sem }()
			req, err := http.NewRequest("POST", fmt.Sprintf("%s/api/v1/admin/media/%s/detect-segments", *apiBase, id), nil)
			if err != nil {
				fail.Add(1)
				fmt.Printf("[%d/%d] %s ERR (req): %v\n", idx+1, len(ids), id, err)
				return
			}
			req.Header.Set("Authorization", "Bearer "+*token)
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				fail.Add(1)
				fmt.Printf("[%d/%d] %s ERR (do): %v\n", idx+1, len(ids), id, err)
				return
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusAccepted {
				ok.Add(1)
				fmt.Printf("[%d/%d] %s 202 — %s\n", idx+1, len(ids), id, title)
			} else {
				fail.Add(1)
				fmt.Printf("[%d/%d] %s HTTP %d — %s\n", idx+1, len(ids), id, resp.StatusCode, title)
			}
		}(i, id, titles[i])
	}
	wg.Wait()
	fmt.Printf("\ndone: %d triggered, %d failed in %s\n", ok.Load(), fail.Load(), time.Since(start).Round(time.Second))
	fmt.Println("note: detection runs async on the server; tail logs to watch progress")
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func loadMediaIDs(ctx context.Context, db *sql.DB, extraFilter string, onlyWithoutChapterSource, onlyWithChapters bool, limit int) ([]string, []string, error) {
	var (
		conds []string
		args  []interface{}
	)
	conds = append(conds, "m.type = 'episode'")

	if onlyWithChapters {
		conds = append(conds, "EXISTS (SELECT 1 FROM chapters c WHERE c.media_item_id = m.id)")
	}
	if onlyWithoutChapterSource {
		conds = append(conds, `NOT EXISTS (
			SELECT 1 FROM media_segments ms
			WHERE ms.media_item_id = m.id
			  AND ms.type = 'intro'
			  AND ms.source = 'mkv_chapter'
		)`)
	}
	if extraFilter != "" {
		conds = append(conds, "("+extraFilter+")")
	}

	q := `
		SELECT m.id, m.title, COALESCE(gp.title, '')
		FROM media_items m
		LEFT JOIN media_items p ON p.id = m.parent_id
		LEFT JOIN media_items gp ON gp.id = p.parent_id
		WHERE ` + strings.Join(conds, " AND ") + `
		ORDER BY gp.title, m.title`
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", limit)
	}

	rows, err := db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, nil, err
	}
	defer rows.Close()

	var ids, titles []string
	for rows.Next() {
		var id, title, series string
		if err := rows.Scan(&id, &title, &series); err != nil {
			return nil, nil, err
		}
		ids = append(ids, id)
		display := title
		if series != "" {
			display = series + " — " + title
		}
		titles = append(titles, display)
	}
	return ids, titles, rows.Err()
}
