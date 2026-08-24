package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// The i18n key guard (frontend/apps/tv/src/test/i18nKeys.test.ts) checks that
// every t("...") in tv, web and mobile resolves against the server catalogue.
// It exists because a raw fallback reached a real television.
//
// It cannot run everywhere. It walks up from its own file to find the repo
// root, and guards itself with `it.skipIf(!CAN_RUN)`; inside the frontend test
// container only `frontend/` is mounted, the root is not found, and all four
// of its cases SKIP - vitest prints "4 skipped" and the suite is green.
// Verified on 2026-08-24: containerised run = 4 skipped, host run = 4 passed.
//
// So the only place it actually executes in the gate is the `tv-audit` step,
// which runs on the HOST. Drop that step from pre-push and the guard stops
// running ANYWHERE while still reporting success from the container - the
// failure mode it was written to prevent, applied to itself.
var (
	prePushTargetRe = regexp.MustCompile(`(?m)^pre-push:[ \t]*(.*)$`)
	tvAuditBodyRe   = regexp.MustCompile(`(?m)^tv-audit:\n((?:\t.*\n)+)`)
)

const i18nGateStep = "tv-audit"

func TestPrePushStillRunsTheI18nKeyGuard(t *testing.T) {
	makefile := filepath.Join(repoRoot, "Makefile")
	body, err := os.ReadFile(makefile)
	if err != nil {
		t.Fatalf("read Makefile: %v", err)
	}
	src := string(body)

	m := prePushTargetRe.FindStringSubmatch(src)
	if m == nil {
		t.Fatal("no `pre-push:` target found in the Makefile - the guard would pass vacuously")
	}
	deps := strings.Fields(m[1])
	if len(deps) < 5 {
		t.Fatalf("pre-push declares only %d prerequisites (%v) - that does not look like the gate, "+
			"and this guard must not vouch for a target it failed to parse", len(deps), deps)
	}
	if !contains(deps, i18nGateStep) {
		t.Fatalf("pre-push no longer depends on %q: the i18n key guard SKIPS in the frontend test "+
			"container (it cannot find the repo root there), so this step is the only place it "+
			"actually runs. Without it the check reports success without having read anything.",
			i18nGateStep)
	}

	// The step must still invoke the key suite, not merely exist under that name.
	tv := tvAuditBodyRe.FindStringSubmatch(src)
	if tv == nil {
		t.Fatalf("no `%s:` recipe found - update this guard alongside the target", i18nGateStep)
	}
	if !strings.Contains(tv[1], "i18nKeys.test.ts") {
		t.Errorf("the %s recipe no longer runs i18nKeys.test.ts:\n%s", i18nGateStep, tv[1])
	}
	// It runs on the host precisely because the container cannot resolve the
	// repo root. A `docker exec` here would silently return to 4 skipped.
	if strings.Contains(tv[1], "docker exec") || strings.Contains(tv[1], "docker run") {
		t.Errorf("the %s recipe now runs in a container, where the i18n guard skips itself "+
			"(4 skipped, exit 0). It has to run on the host:\n%s", i18nGateStep, tv[1])
	}
}

func contains(haystack []string, needle string) bool {
	for _, h := range haystack {
		if h == needle {
			return true
		}
	}
	return false
}
