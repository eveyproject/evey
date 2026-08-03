// Slot-parity gate: every plugin UI slot the server's registry allows must be
// MOUNTED by the clients - and mounted by BOTH web and mobile, because a
// plugin declares its placement once and the product promise is "visible and
// reachable the same way everywhere" (owner's words, 2026-08-03, after
// weather-v2's settings.user.appearance panel shipped invisible on mobile).
//
// Same scanning posture as the field guard: the registry is parsed from the
// domain source, the clients are grepped for the quoted slot name. A slot a
// client renders under a computed name would false-flag here - none does, and
// the literal name IS the greppable contract.
package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// slotAllowlist holds slots knowingly absent somewhere, each with its reason
// and a decision owner. An entry here keeps the gate green WITHOUT making the
// absence invisible - it is the difference between a decision and a hole.
// (home.hero and library.sidebar sat here as "mounted nowhere" for a few
// hours on 2026-08-03; the owner ruled: implement, everywhere.)
var slotAllowlist = map[string]string{}

func registeredSlots(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "internal/domain/plugin/slots.go"))
	if err != nil {
		t.Fatalf("read slots registry: %v", err)
	}
	re := regexp.MustCompile(`"([a-z][a-z0-9.]+)":\s*\{Auth`)
	var out []string
	for _, m := range re.FindAllStringSubmatch(string(b), -1) {
		out = append(out, m[1])
	}
	if len(out) < 5 {
		t.Fatalf("only %d slots parsed from the registry - the parser lost the file, refusing to pass vacuously", len(out))
	}
	return out
}

// The plugin HOST BRIDGE must expose the same surface on web (PluginIframe +
// front-bridge) and mobile (PluginSlot + pluginBridge): an RPC or message
// type present on one side only means every plugin using it is silently
// half-functional on the other platform - the same promise as the slots.
func TestPluginBridgeParityWebMobile(t *testing.T) {
	read := func(rel string) string {
		b, err := os.ReadFile(filepath.Join(repoRoot, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(b)
	}
	extract := func(corpus string, re *regexp.Regexp) map[string]bool {
		set := map[string]bool{}
		for _, m := range re.FindAllStringSubmatch(corpus, -1) {
			set[m[1]] = true
		}
		return set
	}
	diff := func(what string, a, b map[string]bool, aName, bName string) {
		for k := range a {
			if !b[k] {
				t.Errorf("%s %q exists in %s but not in %s - plugins using it are half-functional there", what, k, aName, bName)
			}
		}
	}

	rpcRe := regexp.MustCompile(`case "([a-z]+\.[a-z]+)":`)
	webRPC := extract(read("frontend/apps/web/src/features/plugins/PluginIframe.tsx"), rpcRe)
	mobRPC := extract(read("frontend/apps/mobile/src/features/plugins/PluginSlot.tsx"), rpcRe)
	if len(webRPC) < 4 || len(mobRPC) < 4 {
		t.Fatalf("bridge RPC extraction lost the files (web=%d mobile=%d) - refusing to pass vacuously", len(webRPC), len(mobRPC))
	}
	diff("bridge RPC", webRPC, mobRPC, "web PluginIframe", "mobile PluginSlot")
	diff("bridge RPC", mobRPC, webRPC, "mobile PluginSlot", "web PluginIframe")

	typeRe := regexp.MustCompile(`"(rpc|rpc-result|ready|init|navigate|toast|resize)"`)
	webTypes := extract(read("frontend/apps/web/src/features/plugins/front-bridge.ts"), typeRe)
	mobTypes := extract(read("frontend/apps/mobile/src/features/plugins/pluginBridge.ts"), typeRe)
	diff("bridge message type", webTypes, mobTypes, "web front-bridge", "mobile pluginBridge")
	diff("bridge message type", mobTypes, webTypes, "mobile pluginBridge", "web front-bridge")
}

func TestEveryAllowedSlotIsMountedOnWebAndMobile(t *testing.T) {
	slots := registeredSlots(t)
	web := collectCorpus(t, []string{filepath.Join(repoRoot, "frontend/apps/web/src")}, isClientSource)
	mobile := collectCorpus(t, []string{
		filepath.Join(repoRoot, "frontend/apps/mobile/app"),
		filepath.Join(repoRoot, "frontend/apps/mobile/src"),
	}, isClientSource)

	for _, slot := range slots {
		if _, ok := slotAllowlist[slot]; ok {
			continue
		}
		quoted := `"` + slot + `"`
		onWeb := strings.Contains(web, quoted)
		onMobile := strings.Contains(mobile, quoted)
		switch {
		case !onWeb && !onMobile:
			t.Errorf("slot %q is registered but mounted by NO client - a plugin declaring it appears nowhere, silently; mount it or allowlist the decision", slot)
		case onWeb && !onMobile:
			t.Errorf("slot %q is mounted on web but NOT on mobile - the placement promise is broken (weather-v2 class); mount it in the mobile surface mirroring the web one", slot)
		case onMobile && !onWeb:
			t.Errorf("slot %q is mounted on mobile but NOT on web - same promise, other direction", slot)
		}
	}
}
