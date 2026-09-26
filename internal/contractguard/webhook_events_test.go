package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The webhook forms (web and phone) offer the events of one shared list,
// frontend/packages/client/src/lib/webhookEvents.ts. It must name exactly the
// events the server publishes: the webhook service subscribes to "*" and
// delivers every published event (ADR-0010, 2026-09-23 amendment), so a
// missing name is an event nobody can subscribe to by name, and an extra one
// is a checkbox that never fires - the form offered three of those until
// 2026-09-26.
var (
	eventNameDeclRe = regexp.MustCompile(`EventName\(\) string \{ return "([a-z._]+)" \}`)
	webhookListRe   = regexp.MustCompile(`"([a-z][a-z._]+)",`)
)

func TestWebhookFormListsExactlyThePublishedEvents(t *testing.T) {
	server := map[string]bool{}
	root := filepath.Join(repoRoot, "internal", "domain")
	walkErr := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for _, m := range eventNameDeclRe.FindAllStringSubmatch(string(b), -1) {
			server[m[1]] = true
		}
		return nil
	})
	if walkErr != nil {
		t.Fatalf("walk internal/domain: %v", walkErr)
	}
	if len(server) < 40 {
		t.Fatalf("found only %d event declarations: the scan is broken, not the list", len(server))
	}

	b, err := os.ReadFile(filepath.Join(repoRoot, "frontend", "packages", "client", "src", "lib", "webhookEvents.ts"))
	if err != nil {
		t.Fatalf("read the webhook event list: %v", err)
	}
	listed := map[string]bool{}
	for _, m := range webhookListRe.FindAllStringSubmatch(string(b), -1) {
		listed[m[1]] = true
	}

	var missing, extra []string
	for name := range server {
		if !listed[name] {
			missing = append(missing, name)
		}
	}
	for name := range listed {
		if !server[name] {
			extra = append(extra, name)
		}
	}
	slices.Sort(missing)
	slices.Sort(extra)
	if len(missing) > 0 {
		t.Errorf("published but not offered in the webhook form: %v", missing)
	}
	if len(extra) > 0 {
		t.Errorf("offered in the webhook form but never published: %v", extra)
	}
}
