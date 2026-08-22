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
	"slices"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/evey-project/evey/internal/adapters/driven/postgres"
	httpapi "github.com/evey-project/evey/internal/adapters/driving/http"
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
	backup := postgres.NewestBackup(cfg.Backup.Dir)

	fmt.Println("Evey rollback advisor (read-only)")
	fmt.Println()
	fmt.Printf("  running version   %s\n", buildID().Version)
	fmt.Printf("  running commit    %s (built %s)\n", buildID().Commit, buildID().BuildDate)
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
	fmt.Println("  docker images 'gravityz/evey*' --format '{{.Repository}}:{{.Tag}}\\t{{.CreatedSince}}'")
	fmt.Println()
	if backup == "" {
		// EVEY_VERSION is the variable docker-compose.yml interpolates. `up -d`
		// on its own recreates only the service whose image changed, so the
		// database and the reverse proxy keep running: no `down` needed, and
		// the outage is the one container restart you cannot avoid anyway.
		fmt.Println("  # 2. relaunch on the previous tag (recreates the evey container only)")
		fmt.Println("  EVEY_VERSION=<previous> docker compose up -d")
	} else {
		fmt.Println("  # 2. stop the server, leaving postgres up for the restore")
		fmt.Println("  docker compose stop evey")
		fmt.Println()
		fmt.Println("  # 3. ONLY if the newer version applied migrations, restore the pre-migration DB")
		fmt.Printf("  EVEY_VERSION=<previous> docker compose run --rm evey admin restore-backup %s\n", backup)
		fmt.Println("  #    (or step back one migration at a time with: evey admin migrate down)")
		fmt.Println()
		fmt.Println("  # 4. bring the stack back up on the previous image tag")
		fmt.Println("  EVEY_VERSION=<previous> docker compose up -d")
	}
	fmt.Println()
	fmt.Println("Scripted helper (adapt the image/compose vars to your host): scripts/evey-rollback.sh")
	return nil
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

	// Media library roots: existence + writable by the server identity (the known
	// prod foot-gun where reindex/repair fails with "permission denied").
	res = append(res, checkLibraryPaths(db)...)

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

	// The remote-access gate, checked BEFORE the external tools: a locked-out
	// household is more urgent than a missing binary, and it is the one failure
	// whose symptom ("it works for you, not for me") points nowhere useful.
	// Read through the SAME two sources the server resolves at boot (env, then
	// the persisted panel value) - judging on env alone would report a lockout
	// on every instance configured from the panel.
	res = append(res, checkRemoteAccess(effectiveRemoteAccess(db, cfg), cfg.Server.BaseURL))

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
	if scanErr := db.QueryRow(`SELECT count(*) FROM media_items`).Scan(&dbCount); scanErr != nil {
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

// checkLibraryPaths reports each library root's existence and whether the
// SERVER can write it - the kernel's owner/group/other semantics against PID
// 1's identity, not a naive owner-uid comparison: a gravity:users library
// with g+rw is perfectly repairable by a server running as 1000:100.
func checkLibraryPaths(db *sql.DB) []checkResult {
	uid, gid, groups := serverIdentity()
	var out []checkResult
	for _, p := range libraryPaths(db) {
		fi, err := os.Stat(p)
		if err != nil {
			out = append(out, checkResult{"media " + p, "WARN", "unreadable: " + err.Error(), "check the mount"})
			continue
		}
		st, ok := fi.Sys().(*syscall.Stat_t)
		if !ok {
			out = append(out, checkResult{"media " + p, "ok", "readable", ""})
			continue
		}
		if writableByServer(st, fi.Mode(), uid, gid, groups) {
			out = append(out, checkResult{"media " + p, "ok",
				fmt.Sprintf("writable by the server (uid %d, gid %d)", uid, gid), ""})
			continue
		}
		out = append(out, checkResult{"media " + p, "WARN",
			fmt.Sprintf("not writable by the server (uid %d, groups %v): owner %d:%d mode %o", uid, append([]int{gid}, groups...), st.Uid, st.Gid, fi.Mode().Perm()),
			"repairs will fail; give the server's group write access (chgrp + chmod g+w) or align PUID/PGID with the media owner (docs/configuration.md, \"File ownership\"); ignore if an ACL already grants it"})
	}
	return out
}

// serverIdentity returns the uid/gid/supplementary groups the SERVER runs as.
// `docker exec` shells run as the image's default user (root since the PUID
// entrypoint), so the doctor reads PID 1 - the server itself - and only falls
// back to the PUID/PGID env defaults when /proc/1/status is not readable.
func serverIdentity() (int, int, []int) {
	uid, gid := targetIDs()
	b, err := os.ReadFile("/proc/1/status")
	if err != nil {
		return uid, gid, nil
	}
	return parseProcStatusIDs(string(b), uid, gid)
}

// parseProcStatusIDs extracts the real Uid/Gid and the supplementary Groups
// from a /proc/<pid>/status payload, keeping the fallbacks on parse failure.
func parseProcStatusIDs(s string, fallbackUID, fallbackGID int) (int, int, []int) {
	uid, gid, groups := fallbackUID, fallbackGID, []int(nil)
	for line := range strings.SplitSeq(s, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}
		switch fields[0] {
		case "Uid:":
			if n, err := strconv.Atoi(fields[1]); err == nil {
				uid = n
			}
		case "Gid:":
			if n, err := strconv.Atoi(fields[1]); err == nil {
				gid = n
			}
		case "Groups:":
			for _, f := range fields[1:] {
				if n, err := strconv.Atoi(f); err == nil {
					groups = append(groups, n)
				}
			}
		}
	}
	return uid, gid, groups
}

// writableByServer applies the kernel's classic permission walk (owner, then
// group, then other) for the server identity on a stat result. ACL grants
// beyond the owning group are invisible here - the check errs on the warning
// side and says so.
func writableByServer(st *syscall.Stat_t, mode os.FileMode, uid, gid int, groups []int) bool {
	perm := mode.Perm()
	if int(st.Uid) == uid {
		return perm&0o200 != 0
	}
	if int(st.Gid) == gid || slices.Contains(groups, int(st.Gid)) {
		return perm&0o020 != 0
	}
	return perm&0o002 != 0
}

// libraryPaths flattens every library's jsonb paths array into a []string.
func libraryPaths(db *sql.DB) []string {
	if db == nil {
		return nil
	}
	rows, err := db.Query(`SELECT paths FROM libraries ORDER BY created_at, id`)
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
	if rowsErr := rows.Err(); rowsErr != nil {
		return all // best-effort listing: whatever iterated cleanly still helps the doctor
	}
	return all
}

func checkWritableDir(name, path string, uid, gid int, repair bool) checkResult {
	tryWrite := func() error {
		f := filepath.Join(path, ".evey-doctor-write-test")
		if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
			return err
		}
		if rmErr := os.Remove(f); rmErr != nil {
			// The dir accepted the write: report it writable. A probe file we
			// cannot delete back is odd enough to surface, not enough to fail.
			fmt.Fprintf(os.Stderr, "note: could not clean up %s: %v\n", f, rmErr)
		}
		return nil
	}
	_, statErr := os.Stat(path)
	if statErr == nil && tryWrite() == nil {
		return checkResult{name, "ok", path, ""}
	}
	if repair {
		mkErr := os.MkdirAll(path, 0o755)
		chownErr := os.Chown(path, uid, gid)
		if tryWrite() == nil {
			return checkResult{name, "ok", path, "repaired (created + chowned)"}
		}
		// The repair did not take: say WHY instead of a bare FAIL below.
		if mkErr != nil {
			return checkResult{name, "FAIL", path, "repair failed: " + mkErr.Error()}
		}
		if chownErr != nil {
			return checkResult{name, "FAIL", path, "repair failed: " + chownErr.Error()}
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
		return checkResult{name, "FAIL", detail, "past the 98% mark; ingest/transcode will fail"}, true
	case pct >= 90:
		return checkResult{name, "WARN", detail, "past the 90% mark; free up space before it hits 98% and fails"}, true
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
	body, readErr := io.ReadAll(resp.Body)
	if readErr != nil {
		body = nil // callers key on the status code; an unreadable body reads as empty
	}
	return resp.StatusCode, body, nil
}

func isTTY(f *os.File) bool {
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// checkRemoteAccess reports the one combination that cannot be right by
// accident: a server published at a public URL while the remote-access gate
// refuses every client that is not on the local network (HTTP 403 on every
// route). It is a FAIL rather than a WARN because nothing works for anyone
// outside the house - and the symptom people report ("it works on your phone,
// not on mine") points at the client, not here.
func checkRemoteAccess(ra config.RemoteAccessConfig, baseURL string) checkResult {
	// Same source of truth as the admin panel and the boot log. This used to
	// re-derive "published" inline, which made three independent copies of one
	// rule - and a doctor that disagrees with the panel about whether the
	// household is locked out is worse than no doctor at all.
	published, lockedOut := config.RemoteAccessStatus(&ra, baseURL)
	publicURL := ra.ExternalURL
	if !config.IsPubliclyReachableURL(publicURL) {
		publicURL = baseURL
	}

	switch {
	case lockedOut:
		return checkResult{
			"remote access", "FAIL",
			fmt.Sprintf("DISABLED while published at %s - every client outside your local network gets HTTP 403 on every route", publicURL),
			"turn it on in Admin > Remote access (takes effect immediately), or set EVEY_REMOTE_ACCESS_ENABLED=true",
		}
	case published:
		return checkResult{"remote access", "ok", "enabled, published at " + publicURL, ""}
	case ra.Enabled:
		return checkResult{"remote access", "ok", "enabled (no public URL configured)", ""}
	default:
		return checkResult{"remote access", "ok", "LAN-only (no public URL configured)", ""}
	}
}

// effectiveRemoteAccess reproduces the server's own boot resolution so the
// doctor judges what the server WILL do, not what the environment alone says:
// env defaults, then the value the operator saved in the panel (which wins),
// then the published-URL inference. Falls back to the env view when the DB is
// unreachable - the database check above already reports that.
func effectiveRemoteAccess(db *sql.DB, cfg *config.Config) config.RemoteAccessConfig {
	ra := cfg.RemoteAccess
	if db == nil {
		config.ResolveRemoteAccess(&ra, cfg.Server.BaseURL)
		return ra
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	store := httpapi.NewServerConfigStore(postgres.NewServerConfigRepository(db))
	if raw, err := store.Get(ctx, httpapi.RemoteAccessConfigKey); err == nil && raw != "" {
		if err := json.Unmarshal([]byte(raw), &ra); err != nil {
			// Malformed row: the env view is the honest fallback, and the check
			// below will still describe whatever it decides.
			ra = cfg.RemoteAccess
		}
	}
	config.ResolveRemoteAccess(&ra, cfg.Server.BaseURL)
	return ra
}
