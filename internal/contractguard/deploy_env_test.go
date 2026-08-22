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

// The rollback surface (the helper script and the `evey admin rollback`
// advisor) tells the operator to relaunch the stack with an EVEY_* variable
// in front of `docker compose up -d`. Compose only honours a variable the
// compose file actually interpolates; any other name is silently ignored and
// `up -d` re-launches the SAME image.
//
// That is how the rollback shipped: the script exported EVEY_IMAGE_TAG while
// docker-compose.yml reads ${EVEY_VERSION}. `--apply` stopped the stack and
// brought it back up on the broken version, printing "Done." - discovered, if
// ever, by someone already in the middle of an incident.
//
// This guard is static on purpose: it must fail on a laptop in under a
// second, not on the NAS at 2 a.m.
var composeEnvHandoffRe = regexp.MustCompile(`(EVEY_[A-Z0-9_]+)=\S*\s+(?:\$COMPOSE|\$\{COMPOSE[^}]*\}|docker compose|docker-compose)\b`)

var composeInterpolationRe = regexp.MustCompile(`\$\{(EVEY_[A-Z0-9_]+)`)

func TestComposeEnvHandoffsAreReadByCompose(t *testing.T) {
	interpolated := composeInterpolatedVars(t)
	if len(interpolated) == 0 {
		t.Fatal("no ${EVEY_*} interpolation found in docker-compose*.yml - the guard would pass vacuously")
	}

	type finding struct{ file, line, name string }
	var missing []finding
	var seen int

	for _, path := range deploySurfaceFiles(t) {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			for _, m := range composeEnvHandoffRe.FindAllStringSubmatch(line, -1) {
				seen++
				if interpolated[m[1]] {
					continue
				}
				missing = append(missing, finding{
					file: path,
					line: strings.TrimSpace(line) + " (line " + strconv.Itoa(i+1) + ")",
					name: m[1],
				})
			}
		}
	}

	if seen == 0 {
		t.Fatal("no `EVEY_*=... docker compose` handoff found - the regexp no longer matches the rollback path")
	}
	for _, f := range missing {
		t.Errorf("%s hands %s to compose, but no docker-compose*.yml interpolates it - compose will ignore it and relaunch the current image\n  %s",
			f.file, f.name, f.line)
	}
	if len(missing) > 0 {
		var names []string
		for n := range interpolated {
			names = append(names, n)
		}
		sort.Strings(names)
		t.Logf("variables docker-compose*.yml actually reads: %v", names)
	}
}

func composeInterpolatedVars(t *testing.T) map[string]bool {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(repoRoot, "docker-compose*.yml"))
	if err != nil {
		t.Fatalf("glob compose files: %v", err)
	}
	out := map[string]bool{}
	for _, f := range files {
		body, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		for _, m := range composeInterpolationRe.FindAllStringSubmatch(string(body), -1) {
			out[m[1]] = true
		}
	}
	return out
}

// deploySurfaceFiles lists everything that can tell an operator how to
// relaunch the stack: the shell helpers and the admin CLI that prints the
// manual recipe.
func deploySurfaceFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	for _, pattern := range []string{
		filepath.Join(repoRoot, "scripts", "*.sh"),
		filepath.Join(repoRoot, "cmd", "evey", "*.go"),
		filepath.Join(repoRoot, "docs", "*.md"),
		filepath.Join(repoRoot, "README.md"),
	} {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			t.Fatalf("glob %s: %v", pattern, err)
		}
		out = append(out, matches...)
	}
	return out
}
