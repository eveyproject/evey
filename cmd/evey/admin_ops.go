package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	"github.com/evey-project/evey/internal/config"
)

// -------------------------------------------------------------------------
// list-users -- an email source for shell completion (`reset-password --user`).
// Whoever runs this already has a shell in the container (DB creds, jwt.key),
// so listing emails exposes nothing new. Emails only, one per line, to stdout.
// -------------------------------------------------------------------------

func adminListUsers(args []string) error {
	_, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rows, err := db.QueryContext(ctx, `SELECT email FROM users WHERE email <> '' ORDER BY email`)
	if err != nil {
		return fmt.Errorf("list users: %w", err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var email string
		if err := rows.Scan(&email); err != nil {
			return err
		}
		fmt.Println(email)
	}
	return rows.Err()
}

// -------------------------------------------------------------------------
// rollback -- a READ-ONLY advisor. The container has no access to the Docker
// daemon (by design: mounting the socket would be a big hole), so it cannot
// swap its own image. It prints the running version + schema, the newest
// pre-migration DB backup, and the exact commands to run ON THE HOST.
// -------------------------------------------------------------------------

func adminRollback(args []string) error {
	cfg, db, cleanup, err := openAdminDB()
	if err != nil {
		return err
	}
	defer cleanup()

	schema := "unknown"
	if v, dirty, verr := postgres.CurrentMigrationVersion(db); verr == nil {
		schema = strconv.Itoa(int(v))
		if dirty {
			schema += " (DIRTY)"
		}
	}
	backup := newestPreMigrationBackup(cfg.Backup.Dir)

	fmt.Println("Evey rollback advisor (read-only)")
	fmt.Println()
	fmt.Printf("  running version   %s\n", version)
	fmt.Printf("  schema version    %s\n", schema)
	if backup != "" {
		fmt.Printf("  newest DB backup  %s\n", backup)
	} else {
		fmt.Printf("  newest DB backup  (none in %s)\n", cfg.Backup.Dir)
	}
	fmt.Println()
	fmt.Println("This container cannot roll itself back (no Docker socket, by design).")
	fmt.Println("Run these ON THE HOST, choosing the previous image tag:")
	fmt.Println()
	fmt.Println("  # 1. list the Evey images cached locally, newest first")
	fmt.Println("  docker images 'evey*' --format '{{.Repository}}:{{.Tag}}\\t{{.CreatedSince}}'")
	fmt.Println()
	fmt.Println("  # 2. stop the stack")
	fmt.Println("  docker compose down")
	fmt.Println()
	if backup != "" {
		fmt.Println("  # 3. ONLY if the newer version applied migrations, restore the pre-migration DB")
		fmt.Printf("  docker compose run --rm evey admin restore-backup %s\n", backup)
		fmt.Println("  #    (or step back one migration at a time with: evey admin migrate down)")
		fmt.Println()
	}
	fmt.Println("  # 4. bring the stack back up on the previous image tag")
	fmt.Println("  EVEY_IMAGE_TAG=<previous> docker compose up -d")
	fmt.Println()
	fmt.Println("Scripted helper (adapt the image/compose vars to your host): scripts/evey-rollback.sh")
	return nil
}

// newestPreMigrationBackup returns the most-recently-modified pre-migration dump
// in dir, or "". Sorting by mtime (not name) because the version number in the
// filename breaks lexical order (pre-migration-99 sorts after pre-migration-132).
func newestPreMigrationBackup(dir string) string {
	if dir == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	type cand struct {
		name string
		mod  time.Time
	}
	var cands []cand
	for _, e := range entries {
		n := e.Name()
		if !strings.HasPrefix(n, "pre-migration-") || !strings.HasSuffix(n, ".sql.gz") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		cands = append(cands, cand{n, info.ModTime()})
	}
	if len(cands) == 0 {
		return ""
	}
	sort.Slice(cands, func(i, j int) bool { return cands[i].mod.After(cands[j].mod) })
	return filepath.Join(dir, cands[0].name)
}

// -------------------------------------------------------------------------
// doctor -- aggregate health checks with an optional conservative auto-repair.
// -------------------------------------------------------------------------

type checkResult struct {
	name   string
	status string // "ok" | "WARN" | "FAIL"
	detail string
	hint   string // remediation suggestion (or what --repair did)
}

func adminDoctor(args []string) error {
	fs := flag.NewFlagSet("doctor", flag.ExitOnError)
	repair := fs.Bool("repair", false, "apply safe fixes (create + chown missing writable dirs)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}
	uid, gid := targetIDs()

	var res []checkResult

	// DB + schema. Keep the handle for the checks that read from it.
	var db *sql.DB
	if _, d, cleanup, dberr := openAdminDB(); dberr != nil {
		res = append(res, checkResult{"database", "FAIL", dberr.Error(), "server can't run without the DB"})
	} else {
		db = d
		defer cleanup()
		res = append(res, checkResult{"database", "ok", fmt.Sprintf("%s/%s reachable", cfg.DB.Host, cfg.DB.DBName), ""})
		if v, dirty, verr := postgres.CurrentMigrationVersion(db); verr != nil {
			res = append(res, checkResult{"schema", "WARN", "version unreadable: " + verr.Error(), ""})
		} else if dirty {
			res = append(res, checkResult{"schema", "FAIL", fmt.Sprintf("version %d is DIRTY", v), "a migration failed mid-way; restore a pre-migration backup"})
		} else {
			res = append(res, checkResult{"schema", "ok", fmt.Sprintf("version %d, clean", v), ""})
		}
	}

	// JWT signing key (stat only -- never generate one here).
	if p := cfg.JWT.PrivateKeyPath; p == "" {
		res = append(res, checkResult{"jwt.key", "WARN", "EVEY_JWT_PRIVATE_KEY_PATH unset", ""})
	} else if _, serr := os.Stat(p); serr != nil {
		res = append(res, checkResult{"jwt.key", "WARN", "missing at " + p, "server generates one on next boot (logs everyone out)"})
	} else {
		res = append(res, checkResult{"jwt.key", "ok", p, ""})
	}

	// Meilisearch reachability + index drift.
	res = append(res, checkMeili(cfg, db)...)

	// Media library roots: existence + owned by the server's UID (the known
	// prod foot-gun where reindex/repair fails with "permission denied").
	res = append(res, checkLibraryPaths(db, uid)...)

	// Writable service dirs (create + chown on --repair).
	for _, d := range []struct{ name, path string }{
		{"cache: transcode", os.Getenv("EVEY_TRANSCODING_CACHE_DIR")},
		{"cache: trickplay", os.Getenv("EVEY_TRICKPLAY_CACHE_DIR")},
		{"logs", os.Getenv("EVEY_LOG_DIR")},
		{"backups", cfg.Backup.Dir},
	} {
		if d.path == "" {
			continue
		}
		res = append(res, checkWritableDir(d.name, d.path, uid, gid, *repair))
	}

	// External tools.
	res = append(res, checkTool("ffmpeg", "transcoding needs it"))
	res = append(res, checkTool("mkvgo", "seek-index repair needs it"))

	// Disk pressure on the data/cache/media filesystems.
	seen := map[string]bool{}
	for _, p := range append([]string{"/data", "/cache"}, libraryPaths(db)...) {
		if p == "" || seen[p] {
			continue
		}
		seen[p] = true
		if r, ok := checkDisk(p); ok {
			res = append(res, r)
		}
	}

	printDoctor(res, *repair)

	if countStatus(res, "FAIL") > 0 {
		return fmt.Errorf("doctor: %d check(s) failed", countStatus(res, "FAIL"))
	}
	return nil
}

// checkMeili reports reachability, then compares the index doc count to the DB
// row count (the recurring "search index drift" issue).
func checkMeili(cfg *config.Config, db *sql.DB) []checkResult {
	url := strings.TrimRight(cfg.Meilisearch.URL, "/")
	if url == "" {
		return []checkResult{{"meilisearch", "WARN", "URL unset", ""}}
	}
	code, _, err := httpGet(url+"/health", "", 3*time.Second)
	if err != nil || code != http.StatusOK {
		msg := url + " unreachable"
		if err != nil {
			msg = err.Error()
		}
		return []checkResult{{"meilisearch", "WARN", msg, "search won't work; restart Meili"}}
	}
	out := []checkResult{{"meilisearch", "ok", url + " available", ""}}

	if db == nil {
		return out
	}
	var dbCount int
	if err := db.QueryRow(`SELECT count(*) FROM media_items`).Scan(&dbCount); err != nil {
		return out
	}
	code, body, err := httpGet(url+"/indexes/media_items/stats", cfg.Meilisearch.APIKey, 3*time.Second)
	if err != nil || code != http.StatusOK {
		return out
	}
	var stats struct {
		NumberOfDocuments int `json:"numberOfDocuments"`
	}
	if json.Unmarshal(body, &stats) != nil {
		return out
	}
	if stats.NumberOfDocuments != dbCount {
		hint := "run `evey admin reindex-search`"
		if stats.NumberOfDocuments > dbCount {
			// The index holds MORE than the DB: stale/orphan docs an upsert
			// reindex can't remove. Only a purge-first rebuild shrinks it.
			hint = "run `evey admin reindex-search --purge` (index has stale docs)"
		}
		out = append(out, checkResult{"search index", "WARN",
			fmt.Sprintf("drift: DB has %d, index has %d", dbCount, stats.NumberOfDocuments),
			hint})
	} else {
		out = append(out, checkResult{"search index", "ok", fmt.Sprintf("in sync (%d docs)", dbCount), ""})
	}
	return out
}

// checkLibraryPaths reports each library root's existence and ownership.
func checkLibraryPaths(db *sql.DB, uid int) []checkResult {
	var out []checkResult
	for _, p := range libraryPaths(db) {
		fi, err := os.Stat(p)
		if err != nil {
			out = append(out, checkResult{"media " + p, "WARN", "unreadable: " + err.Error(), "check the mount"})
			continue
		}
		if st, ok := fi.Sys().(*syscall.Stat_t); ok && int(st.Uid) != uid {
			out = append(out, checkResult{"media " + p, "WARN",
				fmt.Sprintf("owned by uid %d, not %d", st.Uid, uid),
				fmt.Sprintf("reindex/repair may fail; on the host: chown -R %d:%d %s", uid, uid, p)})
			continue
		}
		out = append(out, checkResult{"media " + p, "ok", "readable, owned by " + strconv.Itoa(uid), ""})
	}
	return out
}

// libraryPaths flattens every library's jsonb paths array into a []string.
func libraryPaths(db *sql.DB) []string {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT paths FROM libraries`)
	if err != nil {
		return nil
	}
	defer func() { _ = rows.Close() }()
	var all []string
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		var paths []string
		if json.Unmarshal(raw, &paths) == nil {
			all = append(all, paths...)
		}
	}
	return all
}

func checkWritableDir(name, path string, uid, gid int, repair bool) checkResult {
	tryWrite := func() error {
		f := filepath.Join(path, ".evey-doctor-write-test")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			return err
		}
		_ = os.Remove(f)
		return nil
	}
	_, statErr := os.Stat(path)
	if statErr == nil && tryWrite() == nil {
		return checkResult{name, "ok", path, ""}
	}
	if repair {
		_ = os.MkdirAll(path, 0o755)
		_ = os.Chown(path, uid, gid)
		if tryWrite() == nil {
			return checkResult{name, "ok", path, "repaired (created + chowned)"}
		}
	}
	detail := path + " not writable"
	if statErr != nil {
		detail = path + " missing"
	}
	return checkResult{name, "WARN", detail, fmt.Sprintf("run `doctor --repair` (mkdir + chown %d:%d)", uid, gid)}
}

func checkTool(bin, why string) checkResult {
	p, err := exec.LookPath(bin)
	if err != nil {
		return checkResult{bin, "WARN", "not in PATH", why}
	}
	detail := p
	if out, err := exec.Command(bin, "--version").Output(); err == nil {
		if line := strings.SplitN(strings.TrimSpace(string(out)), "\n", 2)[0]; line != "" {
			detail = line
		}
	}
	return checkResult{bin, "ok", detail, ""}
}

func checkDisk(path string) (checkResult, bool) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(path, &st); err != nil {
		return checkResult{}, false
	}
	total := st.Blocks * uint64(st.Bsize)
	if total == 0 {
		return checkResult{}, false
	}
	used := total - st.Bavail*uint64(st.Bsize)
	pct := float64(used) / float64(total) * 100
	name := "disk " + path
	detail := fmt.Sprintf("%.0f%% used", pct)
	switch {
	case pct >= 98:
		return checkResult{name, "FAIL", detail, "nearly full; ingest/transcode will fail"}, true
	case pct >= 90:
		return checkResult{name, "WARN", detail, "getting full"}, true
	default:
		return checkResult{name, "ok", detail, ""}, true
	}
}

func printDoctor(res []checkResult, repaired bool) {
	color := isTTY(os.Stdout)
	green, yellow, red, dim, reset := "", "", "", "", ""
	if color {
		green, yellow, red, dim, reset = "\033[32m", "\033[33m", "\033[31m", "\033[2m", "\033[0m"
	}
	width := 0
	for _, r := range res {
		if len(r.name) > width {
			width = len(r.name)
		}
	}
	fmt.Println()
	var ok, warn, fail int
	for _, r := range res {
		var col, sym string
		switch r.status {
		case "ok":
			col, sym, ok = green, "ok  ", ok+1
		case "WARN":
			col, sym, warn = yellow, "WARN", warn+1
		default:
			col, sym, fail = red, "FAIL", fail+1
		}
		fmt.Printf("  %s%s%s  %-*s  %s\n", col, sym, reset, width, r.name, r.detail)
		if r.hint != "" && r.status != "ok" {
			fmt.Printf("  %s      -> %s%s\n", dim, r.hint, reset)
		}
	}
	fmt.Println()
	fmt.Printf("  %d ok · %d warn · %d fail\n", ok, warn, fail)
	if warn > 0 && !repaired {
		fmt.Printf("  %stip: `evey admin doctor --repair` fixes missing/unwritable dirs%s\n", dim, reset)
	}
	fmt.Println()
}

func countStatus(res []checkResult, status string) int {
	n := 0
	for _, r := range res {
		if r.status == status {
			n++
		}
	}
	return n
}

func targetIDs() (int, int) {
	uid, gid := 1000, 1000
	if v := os.Getenv("PUID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			uid = n
		}
	}
	if v := os.Getenv("PGID"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			gid = n
		}
	}
	return uid, gid
}

func httpGet(url, bearer string, timeout time.Duration) (int, []byte, error) {
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return 0, nil, err
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := (&http.Client{Timeout: timeout}).Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, body, nil
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
