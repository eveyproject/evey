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
// It is applied by hand, per method. A method that forgets it looks exactly
// like every other one until the day the database is slow: on
// 2026-08-23, between 17:28 and 17:49, six background readers on the
// production box timed out together. They failed cleanly and retried BECAUSE
// their repository applied that helper. A file without it would have held its
// connection for the whole window instead.
//
// So: any EXPORTED method of the postgres adapter that executes SQL must
// mention the helper.
//
// This used to be checked per FILE, and that was too coarse to catch what
// actually happens. user_repo.go mentions the helper sixteen times, so the
// file passed - while TouchLastLogin, on the login path, ran with no bound at
// all (found 2026-09-06, along with the per-batch case in
// BackfillEmptySortTitles). One forgetful method in a diligent file was
// invisible.
//
// Exported only, deliberately: the nineteen unexported helpers that run SQL
// (fetchTopMedia, queryMediaItems, upsertMediaFile(tx)...) are called from an
// already-bounded exported method and inherit its context. Demanding a second
// bound there would be noise, and noise is how a guard becomes a mute button.
//
// Still coarse in one way: it cannot prove the helper wraps the RIGHT call.
var (
	// A call, not an interface method declaration: dbExecutor in txunit.go
	// lists these three names with no receiver and executes nothing.
	sqlCallRe = regexp.MustCompile(`\.\s*(QueryRowContext|QueryContext|ExecContext)\(`)
	// `func Name(` or `func (r *Repo) Name(` with Name exported.
	exportedFuncRe = regexp.MustCompile(`^func\s+(\([^)]*\)\s*)?[A-Z]\w*\(`)
)

// Methods that run SQL without the helper, keyed "file.go:MethodName", each
// with the reason it is allowed to.
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
		lines := strings.Split(string(body), "\n")
		// Slice the file into top-level func blocks: a block runs from one
		// `func ` at column zero to the next.
		var starts []int
		for i, l := range lines {
			if strings.HasPrefix(l, "func ") {
				starts = append(starts, i)
			}
		}
		starts = append(starts, len(lines))
		for i := 0; i+1 < len(starts); i++ {
			head := lines[starts[i]]
			// Unexported helpers inherit the bounded context of the exported
			// method that calls them - see the note at the top of this file.
			if !exportedFuncRe.MatchString(head) {
				continue
			}
			block := strings.Join(lines[starts[i]:starts[i+1]], "\n")
			if !sqlCallRe.MatchString(block) {
				continue
			}
			withSQL++
			if strings.Contains(block, "withTimeout(") {
				bounded++
				continue
			}
			key := name + ":" + methodName(head)
			if _, excused := postgresTimeoutExceptions[key]; excused {
				usedException[key] = true
				continue
			}
			missing = append(missing, key)
		}
	}

	// 444 exported methods ran SQL when this was tightened (2026-09-06). A
	// number far below that means the slicing or the detector broke and the
	// guard is passing vacuously.
	if withSQL < 300 {
		t.Fatalf("only %d exported methods in the postgres adapter look like they run SQL - the "+
			"detector has drifted and this guard would pass vacuously", withSQL)
	}
	if bounded == 0 {
		t.Fatal("no method mentions withTimeout - the helper was renamed or removed; update this guard")
	}
	for _, key := range missing {
		t.Errorf("%s runs SQL without withTimeout: on a database that stops answering, that call "+
			"parks its goroutine on a connection with no deadline - nothing else in the stack "+
			"bounds it (no statement_timeout in the DSN). Wrap the ctx like the other %d methods do, "+
			"or add it to postgresTimeoutExceptions with the reason.", key, bounded)
	}
	for key := range postgresTimeoutExceptions {
		if !usedException[key] {
			t.Errorf("postgresTimeoutExceptions still lists %s, which no longer needs it - remove "+
				"the entry instead of leaving it to excuse the next method", key)
		}
	}
}

// methodName pulls "TouchLastLogin" out of "func (r *UserRepository) TouchLastLogin(ctx ...".
func methodName(head string) string {
	rest := strings.TrimPrefix(head, "func ")
	if strings.HasPrefix(rest, "(") {
		if end := strings.Index(rest, ")"); end >= 0 {
			rest = strings.TrimSpace(rest[end+1:])
		}
	}
	if open := strings.Index(rest, "("); open >= 0 {
		rest = rest[:open]
	}
	return strings.TrimSpace(rest)
}
