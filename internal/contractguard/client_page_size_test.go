package contractguard

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// A client asking for more rows than the server will ever serve does not get an
// error - it gets a SHORT LIST that looks complete.
//
// clampLimitOffset (grpcapi/helpers.go) replaces any limit above maxPageSize
// with the handler's own default, and clampPagination caps at maxPageSize.
// Neither says a word. Three screens shipped on that misunderstanding and were
// only found on 2026-08-08, by reading the server rather than the screen:
//
//   - the web photo grid asked for 5000 and rendered 100 of a 7199-photo
//     library, under a header announcing the real total;
//   - the music tab asked for 200 and made every track past the 200th
//     unreachable;
//   - the admin activity export asked for 10000 and wrote a file holding 500
//     of 6109 audit rows, named as if it were the whole log.
//
// None of them failed a test: every assertion was "the list renders", and it
// did. So the guard is on the DATA - the literal the client sends - because
// that is the only place the mistake is visible before a user hits it.
//
// A list bigger than one page is not forbidden. It has to be PAGED (cursor or
// offset), which is what every fix above did. What is forbidden is asking for a
// number the server silently refuses.

// Where clients live. Generated clients are excluded: they carry no literals of
// their own, they forward whatever the caller passes.
var clientRoots = []string{
	"frontend/apps/web/src",
	"frontend/apps/mobile/app",
	"frontend/apps/mobile/src",
	"frontend/apps/tv/app",
	"frontend/apps/tv/src",
	"frontend/packages/api/src",
	"frontend/packages/store/src",
}

// Every spelling a page size takes on the wire: the gateway maps query params
// (`?limit=`), grpc-gateway nests pagination (`"pagination.pageSize"`), and the
// generated clients take plain object keys.
var pageSizeLiteral = regexp.MustCompile(`(?:\blimit:\s*|"pagination\.pageSize":\s*|\bpageSize:\s*|[?&]limit=)([0-9_]+)`)

// Inline literals are only half of it. The TV slideshow held its 1000 in a
// named constant and passed it through a helper, so the first version of this
// guard walked straight past a screen that was showing 100 photos of a
// 228-photo album. A name ending in _LIMIT or _PAGE_SIZE is a per-request page
// size and is pinned the same way.
//
// Deliberately NOT matched: names like EXPORT_MAX_ROWS, which bound a total
// across MANY pages rather than one request - 50000 is correct there.
var pageSizeConst = regexp.MustCompile(`\b[A-Za-z_][A-Za-z0-9_]*(?:_LIMIT|_PAGE_SIZE)\s*=\s*([0-9_]+)`)

func serverMaxPageSize(t *testing.T) int {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "internal/adapters/driving/grpcapi/helpers.go"))
	if err != nil {
		t.Fatalf("read grpcapi helpers: %v", err)
	}
	m := regexp.MustCompile(`const maxPageSize = (\d+)`).FindStringSubmatch(string(b))
	if m == nil {
		t.Fatal("grpcapi/helpers.go: maxPageSize not found as /const maxPageSize = N/ - if it moved, update this guard so the pin keeps holding")
	}
	n, err := strconv.Atoi(m[1])
	if err != nil {
		t.Fatalf("parse maxPageSize %q: %v", m[1], err)
	}
	return n
}

func TestClientsNeverAskForMoreRowsThanTheServerServes(t *testing.T) {
	maxPageSize := serverMaxPageSize(t)

	type offence struct {
		file string
		line int
		want int
	}
	var offences []offence

	for _, root := range clientRoots {
		dir := filepath.Join(repoRoot, root)
		err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" || d.Name() == "generated" {
					return fs.SkipDir
				}
				return nil
			}
			ext := filepath.Ext(path)
			if ext != ".ts" && ext != ".tsx" {
				return nil
			}
			// Test files may exercise absurd values on purpose - they never
			// reach a server.
			if strings.Contains(d.Name(), ".test.") {
				return nil
			}
			b, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			for i, line := range strings.Split(string(b), "\n") {
				for _, re := range []*regexp.Regexp{pageSizeLiteral, pageSizeConst} {
					for _, m := range re.FindAllStringSubmatch(line, -1) {
						n, convErr := strconv.Atoi(strings.ReplaceAll(m[1], "_", ""))
						if convErr != nil || n <= maxPageSize {
							continue
						}
						rel, _ := filepath.Rel(repoRoot, path)
						offences = append(offences, offence{file: rel, line: i + 1, want: n})
					}
				}
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
	}

	for _, o := range offences {
		t.Errorf("%s:%d asks for %d rows, but the server serves at most %d and silently reduces the rest - the screen would render a short list as if it were complete. Page it (cursor or offset) instead of asking for everything.",
			o.file, o.line, o.want, maxPageSize)
	}
}
