package httpapi

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"time"
)

// logSearchEntry is one parsed, classified log line surfaced to the admin
// debug UI. Raw holds the original JSON line so the client can always fall
// back to the source. Origin is "server" today; it is emitted now so a future
// client-log ingestion (web-player/mobile/tv) slots in without a schema break.
type logSearchEntry struct {
	Time      string         `json:"time"`
	Level     string         `json:"level"`
	Msg       string         `json:"msg"`
	Component string         `json:"component,omitempty"`
	Where     string         `json:"where"`
	Source    string         `json:"source"`
	Origin    string         `json:"origin"`
	RequestID string         `json:"requestId,omitempty"`
	TraceID   string         `json:"traceId,omitempty"`
	MediaID   string         `json:"mediaId,omitempty"`
	SessionID string         `json:"sessionId,omitempty"`
	UserID    string         `json:"userId,omitempty"`
	Fields    map[string]any `json:"fields,omitempty"`
	Raw       string         `json:"raw"`
}

// logSearchFilter is the parsed, validated query. Zero values mean "unset"
// (no predicate) so an empty filter matches everything within the scan budget.
type logSearchFilter struct {
	Q         string
	Levels    map[string]bool // normalized upper-case (ERROR/WARN/INFO/DEBUG); empty = any
	Where     string
	Source    string
	SessionID string
	UserID    string
	MediaID   string
	RequestID string
	Since     time.Time // zero = unbounded
	Until     time.Time // zero = unbounded
	Limit     int
	Offset    int
	// HideAccess drops the gRPC access-log lines of RPCs that succeeded
	// ("grpc unary"/"grpc stream" with status OK). On a live instance they are
	// four lines in five - heartbeats and dashboard polls - and bury the
	// application log the operator came for. Failed RPCs stay visible.
	HideAccess bool
}

type logSearchResult struct {
	Entries   []logSearchEntry `json:"entries"`
	Total     int              `json:"total"`
	Scanned   int              `json:"scanned"`
	Truncated bool             `json:"truncated"`
}

// Scan/retain budgets keep a search bounded on a busy instance with large
// rotated logs. We scan newest files first; once the line budget is hit we
// stop and flag truncated so the UI can tell the operator to narrow the query.
//
// Lines OUTSIDE an explicit [Since,Until] window are skipped for free (they
// are not the work the operator asked for) and only bounded by the much
// larger read budget. Without that, a verbose day - a busy instance writes
// well past logScanLineBudget lines a day - burns the whole scan budget on
// recent noise and makes any older window return nothing at all.
// logRetainCap sizes the retention window RELATIVE TO THE REQUESTED PAGE
// (offset+limit), not as a flat ceiling on the whole result set. A flat cap
// looks harmless but silently breaks deep pagination: the search reports
// total=N while every page past the cap comes back EMPTY, so a line that IS
// on disk cannot be produced as evidence. That is what blocked a real prod
// investigation on 2026-07-28.
//
// logRetainCapMax bounds what a single request may hold in memory, and the
// offset query parameter is clamped to it (parseLogSearchFilter) so the API
// never accepts an offset it cannot actually serve. The common case
// (offset=0, limit=200) now retains 200 entries where the old flat cap
// retained 5000 - strictly less memory, and every accepted page is reachable.
const (
	logScanLineBudget = 300_000
	logReadLineBudget = 5_000_000
	logRetainCap      = 5_000
	logRetainCapMax   = 50_000
	logScannerMaxLine = 2 << 20 // 2 MiB: ffmpeg arg lines can be long
)

// searchLogs scans the rotating evey-*.log files in dir (newest first),
// parses each line as a slog JSON record, applies the filter and returns the
// matches sorted newest-first with offset/limit applied. Total counts every
// match found within the scan budget (a lower bound when Truncated is true).
func searchLogs(dir string, f logSearchFilter) (logSearchResult, error) {
	files, err := logFilesNewestFirst(dir)
	if err != nil {
		return logSearchResult{}, err
	}

	keep := newestKeeper{cap: retainWindow(f)}
	var (
		total   int
		scanned int
		read    int
		trunc   bool
	)
	windowed := !f.Since.IsZero() || !f.Until.IsZero()

scan:
	// Files are processed newest-first; within a file we walk lines newest-first
	// (bottom-up) so that when the scan budget is hit, the budget was spent on
	// the freshest data - exactly what a debug view wants.
	for _, path := range files {
		// A file last written before Since holds nothing the window wants.
		// An unreadable mtime is never a reason to skip: fall through and
		// let the line filter decide.
		if mt, ok := fileModTime(path); ok && !f.Since.IsZero() && mt.Before(f.Since) {
			continue
		}
		lines, err := readFileLines(path)
		if err != nil {
			continue
		}
		for _, raw := range slices.Backward(lines) {
			if read >= logReadLineBudget {
				trunc = true
				break scan
			}
			read++
			if scanned >= logScanLineBudget {
				trunc = true
				break scan
			}
			line := strings.TrimSpace(raw)
			if line == "" {
				continue
			}
			// Out-of-window lines cost nothing but the read budget, so a
			// verbose recent stretch can't starve an older window.
			if windowed && !lineInWindow(line, f) {
				continue
			}
			scanned++
			entry, ok := parseLogLine(line)
			if !ok {
				continue
			}
			if !matchLog(entry, f) {
				continue
			}
			total++
			keep.add(entry)
		}
	}

	// Newest first: slog "time" is RFC3339Nano and lexicographically sortable.
	matches := keep.entries
	sort.SliceStable(matches, func(i, j int) bool {
		return matches[i].Time > matches[j].Time
	})

	lo := min(f.Offset, len(matches))
	hi := lo + f.Limit
	if f.Limit <= 0 || hi > len(matches) {
		hi = len(matches)
	}

	return logSearchResult{
		Entries:   matches[lo:hi],
		Total:     total,
		Scanned:   scanned,
		Truncated: trunc,
	}, nil
}

// retainWindow sizes the retention buffer for one request: exactly the page
// asked for (offset+limit), so any accepted offset is serveable. An unpaged
// caller (Limit <= 0 - internal callers, not the HTTP layer, which always
// defaults it) keeps the historical flat window.
func retainWindow(f logSearchFilter) int {
	n := logRetainCap
	if f.Limit > 0 {
		n = f.Offset + f.Limit
	}
	return max(1, min(n, logRetainCapMax))
}

// newestKeeper retains the `cap` newest entries it is shown, whatever order
// they arrive in. The scan walks files newest-first and lines bottom-up, so
// arrivals are ALMOST sorted - but log files overlap around a rotation, and a
// keeper that just took the first `cap` arrivals would drop a genuinely newer
// entry that shows up late. Since displacements only happen at those
// boundaries, a linear scan for the current oldest is cheaper in practice than
// the bookkeeping of a heap, and far easier to read.
type newestKeeper struct {
	cap     int
	entries []logSearchEntry
	oldest  int // index of the oldest retained entry; valid only once full
}

func (k *newestKeeper) add(e logSearchEntry) {
	if len(k.entries) < k.cap {
		k.entries = append(k.entries, e)
		if len(k.entries) == k.cap {
			k.reindexOldest()
		}
		return
	}
	if e.Time <= k.entries[k.oldest].Time {
		return
	}
	k.entries[k.oldest] = e
	k.reindexOldest()
}

func (k *newestKeeper) reindexOldest() {
	k.oldest = 0
	for i := 1; i < len(k.entries); i++ {
		if k.entries[i].Time < k.entries[k.oldest].Time {
			k.oldest = i
		}
	}
}

// readFileLines loads every line of a log file. Files are size-bounded by the
// rotating writer (EVEY_LOG_MAX_SIZE_MB, default 50), so reading one whole file
// at a time to enable newest-first (reverse) iteration is acceptable.
func readFileLines(path string) ([]string, error) {
	fh, err := os.Open(path) //nolint:gosec // path is derived from a fixed admin log dir listing.
	if err != nil {
		return nil, err
	}
	defer fh.Close()
	sc := bufio.NewScanner(fh)
	sc.Buffer(make([]byte, 0, 64*1024), logScannerMaxLine)
	var lines []string
	for sc.Scan() {
		lines = append(lines, sc.Text())
	}
	return lines, sc.Err()
}

// logFilesNewestFirst lists evey-*.log files in dir sorted by mtime desc, so a
// bounded scan reads the freshest data first.
func logFilesNewestFirst(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	type fileInfo struct {
		path  string
		mtime time.Time
	}
	var files []fileInfo
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasPrefix(name, "evey-") || !strings.HasSuffix(name, ".log") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		files = append(files, fileInfo{path: filepath.Join(dir, name), mtime: info.ModTime()})
	}
	sort.Slice(files, func(i, j int) bool { return files[i].mtime.After(files[j].mtime) })
	out := make([]string, len(files))
	for i, f := range files {
		out[i] = f.path
	}
	return out, nil
}

// parseLogLine turns a raw slog JSON line into a classified entry. Non-JSON
// lines (e.g. a stray panic dump) are dropped from search results.
func parseLogLine(line string) (logSearchEntry, bool) {
	line = redactLogLine(line)
	var rec map[string]any
	if err := json.Unmarshal([]byte(line), &rec); err != nil {
		return logSearchEntry{}, false
	}

	e := logSearchEntry{Raw: line, Origin: "server"}
	e.Time = strField(rec, "time")
	e.Level = strings.ToUpper(strField(rec, "level"))
	e.Msg = strField(rec, "msg")
	e.Component = strField(rec, "component")
	e.RequestID = strField(rec, "request_id")
	e.TraceID = strField(rec, "trace_id")
	// Normalize the well-known correlation ids across their snake/camel variants.
	e.MediaID = firstField(rec, "media_id", "mediaId", "mediaID")
	e.SessionID = firstField(rec, "session_id", "sessionID")
	e.UserID = firstField(rec, "user_id", "userID")

	e.Where, e.Source = classifyLog(e.Component, e.Msg, strField(rec, "engine"))

	// Remaining attrs (everything that is not a promoted top-level field) go
	// into Fields so the UI can render the full record without us enumerating
	// every possible key.
	promoted := map[string]bool{
		"time": true, "level": true, "msg": true, "component": true,
		"request_id": true, "trace_id": true,
	}
	fields := make(map[string]any, len(rec))
	for k, v := range rec {
		if promoted[k] {
			continue
		}
		fields[k] = v
	}
	if len(fields) > 0 {
		e.Fields = fields
	}
	return e, true
}

func strField(rec map[string]any, key string) string {
	if v, ok := rec[key].(string); ok {
		return v
	}
	return ""
}

func firstField(rec map[string]any, keys ...string) string {
	for _, k := range keys {
		if v := strField(rec, k); v != "" {
			return v
		}
	}
	return ""
}

// lineInWindow is the cheap [Since,Until] pre-filter run BEFORE the full JSON
// parse, so skipping a line the operator did not ask for stays near-free. It
// reads the leading "time" field slog always writes first; a line whose stamp
// can't be read is kept (the real parse + matchLog decide).
func lineInWindow(line string, f logSearchFilter) bool {
	ts, ok := logLineTime(line)
	if !ok {
		return true
	}
	if !f.Since.IsZero() && ts.Before(f.Since) {
		return false
	}
	if !f.Until.IsZero() && ts.After(f.Until) {
		return false
	}
	return true
}

// logLineTime pulls the RFC3339 "time" value out of a raw slog JSON line
// without unmarshalling the whole record.
func logLineTime(line string) (time.Time, bool) {
	const key = `"time":"`
	_, rest, found := strings.Cut(line, key)
	if !found {
		return time.Time{}, false
	}
	raw, _, found := strings.Cut(rest, `"`)
	if !found {
		return time.Time{}, false
	}
	ts, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, false
	}
	return ts, true
}

// fileModTime is the file's last write; ok is false when it can't be stat'd,
// so callers keep the file rather than skip it on a missing mtime.
func fileModTime(path string) (time.Time, bool) {
	info, err := os.Stat(path)
	if err != nil {
		return time.Time{}, false
	}
	return info.ModTime(), true
}

// isAccessLog tells a per-RPC access line (emitted by the gRPC logging
// interceptor) that reports success apart from everything else. Mirrored on
// the client for the live tail in logsClassify.ts.
func isAccessLog(e logSearchEntry) bool {
	if e.Msg != "grpc unary" && e.Msg != "grpc stream" {
		return false
	}
	st, _ := e.Fields["status"].(string)
	return st == "OK"
}

func matchLog(e logSearchEntry, f logSearchFilter) bool {
	if len(f.Levels) > 0 && !f.Levels[e.Level] {
		return false
	}
	if f.HideAccess && isAccessLog(e) {
		return false
	}
	if f.Where != "" && e.Where != f.Where {
		return false
	}
	if f.Source != "" && e.Source != f.Source {
		return false
	}
	if f.SessionID != "" && e.SessionID != f.SessionID {
		return false
	}
	if f.UserID != "" && e.UserID != f.UserID {
		return false
	}
	if f.MediaID != "" && e.MediaID != f.MediaID {
		return false
	}
	if f.RequestID != "" && e.RequestID != f.RequestID {
		return false
	}
	if !f.Since.IsZero() || !f.Until.IsZero() {
		ts, err := time.Parse(time.RFC3339Nano, e.Time)
		if err != nil {
			return false
		}
		if !f.Since.IsZero() && ts.Before(f.Since) {
			return false
		}
		if !f.Until.IsZero() && ts.After(f.Until) {
			return false
		}
	}
	if f.Q != "" {
		// Case-insensitive substring over the raw line so the operator can grep
		// any attr value (paths, error text, ids) without a dedicated field.
		if !strings.Contains(strings.ToLower(e.Raw), strings.ToLower(f.Q)) {
			return false
		}
	}
	return true
}
