package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// Nothing outside this repository bounds a Postgres query.
//
// The DSN (internal/config) carries no statement_timeout and no lock_timeout,
// and the pool only limits how many connections exist and how long they live -
// not how long one query may take. The single thing standing between a
// database that stops answering and a goroutine parked forever on a connection
// is the package helper:
//
//	func withTimeout(ctx, d) { if ctx has a deadline -> keep it; else -> 30s }
//
// It is applied per repository file, by hand. A new file that forgets it looks
// exactly like every other file until the day the database is slow: on
// 2026-08-23, between 17:28 and 17:49, six background readers on the
// production box timed out together. They failed cleanly and retried BECAUSE
// their repository applied that helper. A file without it would have held its
// connection for the whole window instead.
//
// So: any file in the postgres adapter that EXECUTES SQL must mention the
// helper. This is deliberately a coarse check - it cannot prove the helper
// wraps the right call - but it catches the case that actually happens, which
// is a file that never heard of it.
var (
	// A call, not an interface method declaration: dbExecutor in txunit.go
	// lists these three names with no receiver and executes nothing.
	sqlCallRe = regexp.MustCompile(`\.\s*(QueryRowContext|QueryContext|ExecContext)\(`)
)

// Files that run SQL without the helper, each with the reason it is allowed to.
var postgresTimeoutExceptions = map[string]string{}

func TestPostgresRepositoriesBoundTheirQueries(t *testing.T) {
	dir := filepath.Join(repoRoot, "internal", "adapters", "driven", "postgres")
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatalf("glob postgres adapter: %v", err)
	}

	usedException := map[string]bool{}
	var withSQL, bounded int
	var missing []string

	for _, path := range paths {
		name := filepath.Base(path)
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		body, readErr := os.ReadFile(path)
		if readErr != nil {
			t.Fatalf("read %s: %v", path, readErr)
		}
		src := string(body)
		if !sqlCallRe.MatchString(src) {
			continue
		}
		withSQL++
		if strings.Contains(src, "withTimeout(") {
			bounded++
			continue
		}
		if _, excused := postgresTimeoutExceptions[name]; excused {
			usedException[name] = true
			continue
		}
		missing = append(missing, name)
	}

	if withSQL < 40 {
		t.Fatalf("only %d files in the postgres adapter look like they run SQL - the detector has "+
			"drifted and this guard would pass vacuously", withSQL)
	}
	if bounded == 0 {
		t.Fatal("no file mentions withTimeout - the helper was renamed or removed; update this guard")
	}
	for _, name := range missing {
		t.Errorf("%s runs SQL without withTimeout: on a database that stops answering, that call "+
			"parks its goroutine on a connection with no deadline - nothing else in the stack "+
			"bounds it (no statement_timeout in the DSN). Wrap the ctx like the other %d files do, "+
			"or add the file to postgresTimeoutExceptions with the reason.", name, bounded)
	}
	for name := range postgresTimeoutExceptions {
		if !usedException[name] {
			t.Errorf("postgresTimeoutExceptions still lists %s, which no longer needs it - remove "+
				"the entry instead of leaving it to excuse the next file", name)
		}
	}
}
