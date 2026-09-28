package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// Watched-state invalidation guard: marking a title watched must refresh every
// row that depends on it, and that list lives in ONE place
// (packages/client/src/lib/invalidateWatchedState.ts). Eight hand-rolled copies
// had drifted from it by 2026-09-28: none refreshed Up Next or Continue
// Watching, the phone's title page aimed at a popular-media key no query uses,
// and the phone's bulk marks refreshed nothing. Any client line that
// invalidates one of those keys outside the shared helper is flagged.
// Limit: a path that refreshes NOTHING (the bulk-mark case) has no line to flag.

const watchedStateHelper = "frontend/packages/client/src/lib/invalidateWatchedState.ts"

var watchedStateKeyRe = regexp.MustCompile(`playback/history|WatchHistoryQueryKey|"/api/v1/popular"|playback/next-up|playback/resume|startsWith\("/api/v1/feeds/"\)`)

// watchedStateFindings reports the lines of src that invalidate a
// watched-sensitive key by hand. A multi-line predicate is caught by its
// startsWith("/api/v1/feeds/") line.
func watchedStateFindings(rel, src string) []string {
	var out []string
	lines := strings.Split(src, "\n")
	for i, line := range lines {
		if !watchedStateKeyRe.MatchString(line) {
			continue
		}
		window := line
		if i > 0 {
			window = lines[i-1] + "\n" + line
		}
		if i > 3 {
			window = strings.Join(lines[i-4:i+1], "\n")
		}
		if strings.Contains(window, "invalidateQueries") {
			out = append(out, rel+":"+strconv.Itoa(i+1)+": "+strings.TrimSpace(line))
		}
	}
	return out
}

func TestWatchedStateInvalidationGuardFires(t *testing.T) {
	src := "const f = () => {\n  void qc.invalidateQueries({\n    predicate: (q) => {\n      const k = q.queryKey?.[0];\n      return k.startsWith(\"/api/v1/feeds/\");\n    },\n  });\n  void qc.invalidateQueries({ queryKey: getPlaybackServiceGetWatchHistoryQueryKey() });\n};\n"
	if got := watchedStateFindings("fixture.ts", src); len(got) != 2 {
		t.Fatalf("findings = %v, want the predicate and the history key", got)
	}
	if got := watchedStateFindings("fixture.ts", `const k = getPlaybackServiceGetWatchHistoryQueryKey();`); len(got) != 0 {
		t.Fatalf("reading the key without invalidating is fine, got %v", got)
	}
}

func TestWatchedStateIsInvalidatedThroughTheSharedHelper(t *testing.T) {
	var findings []string
	scanned := 0
	for _, dir := range []string{"frontend/apps/web/src", "frontend/apps/mobile/app", "frontend/apps/mobile/src", "frontend/apps/tv/app", "frontend/apps/tv/src", "frontend/apps/tv-web/src", "frontend/packages/client/src"} {
		root := filepath.Join(repoRoot, dir)
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if d.Name() == "node_modules" {
					return filepath.SkipDir
				}
				return nil
			}
			if !isClientSource(path) {
				return nil
			}
			rel := strings.TrimPrefix(path, repoRoot+"/")
			if rel == watchedStateHelper {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			scanned++
			findings = append(findings, watchedStateFindings(rel, string(b))...)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
	if scanned < 100 {
		t.Fatalf("scanned %d client files, want hundreds - the walk has drifted", scanned)
	}
	sort.Strings(findings)
	if len(findings) > 0 {
		t.Errorf("watched-state keys invalidated by hand - call invalidateWatchedState (@evey/client) instead:\n  %s",
			strings.Join(findings, "\n  "))
	}
}
