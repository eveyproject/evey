// Admin-gate guard: a web surface that calls an admin-only RPC must decide who
// sees it. The bug class it catches shipped seven times at once on the
// collections screens (found 2026-08-04): Create/Update/Delete/AddItem are all
// RoleAdmin server-side, yet every viewer was shown the buttons. Nothing
// crashed and no test failed - the click simply came back 403 after the person
// had committed to the gesture, which is the worst place to learn it.
//
// The rule: any non-generated web file that reaches an admin-only RPC, directly
// or through one of the app's own hook wrappers, must be classified. Either it
// reads isAdmin itself, or it is declared here as gated by the surfaces that
// mount it - with the reason written down, the same posture as the RBAC guard
// that forces every RPC to be classified.
//
// Honest limits:
//   - web only. Mobile and TV reach the same RPCs through eveyFetch and a URL
//     string, which this symbol matching cannot see; covering them needs the
//     proto http annotations and is not done here.
//   - "the file reads isAdmin" is not "the right control is gated". It stops
//     the ungated surface, not a misplaced condition - that stays the job of
//     the per-surface tests.
package contractguard

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// gatedByCaller lists files that legitimately touch an admin-only RPC without
// reading isAdmin, because the surfaces that mount them do. Each entry names
// those surfaces, so a reader can check the claim.
var gatedByCaller = map[string]string{
	"features/collection/api.ts": "hook wrappers, no UI of their own; every caller is classified below",
	"features/collection/AddToCollectionModal.tsx": "picker with no entry point of its own - mounted behind isAdmin by " +
		"MediaDetailActions, MediaCardWithModals (menu entry gated in MediaCardContextMenu) and the two bulk bars " +
		"(button gated in BulkActionBar)",
	"features/library/MediaCardWithModals.tsx": "mounts the picker only; the menu entry that opens it is gated in MediaCardContextMenu",
	"features/collection/SmartCollectionDialog.tsx": "dialog with no entry point of its own - opened from the collections " +
		"page, whose create and edit controls are gated there",
	"features/collection/CollectionDialog.tsx": "same: the create button and the per-card pencil that open it are both " +
		"gated on the collections page",
	"features/playlist/api.ts": "playlist RPCs are RoleUser; present only because the file sits next to collection api.ts",
	"features/admin/":          "the whole admin area is behind an admin-only route",
	"routes/_auth/admin/":      "the whole admin area is behind an admin-only route",
	"features/plugins/PluginSlot.tsx": "renders plugin-declared UI; the admin RPCs it can reach are gated by the " +
		"plugin's own manifest permissions, checked server-side per call",
}

// mustGate pins the surfaces that reach an admin-only RPC through a callback
// prop instead of a symbol. The scan above cannot see those - it found the
// collections pages but not the bulk bar, whose button calls AddCollectionItem
// through onAddToCollection - so they are named here and checked for a gate.
var mustGate = map[string]string{
	"components/BulkActionBar.tsx":              "its add-to-collection button reaches AddCollectionItem via onAddToCollection",
	"features/library/MediaCardContextMenu.tsx": "its edit-metadata and add-to-collection entries reach admin-only surfaces via callbacks",
	"features/library/MediaDetailActions.tsx":   "its add-to-collection button mounts the picker, and it mounts the metadata editor",
}

var adminRPCRe = regexp.MustCompile(`"/evey\.v1\.(\w+)/(\w+)":\s*RoleAdmin`)

// adminClientSymbols turns the RBAC map into the client function names orval
// generates for those RPCs: CollectionService/DeleteCollection becomes
// collectionServiceDeleteCollection (and its use... hook, which shares the
// prefix, so matching the bare name catches both).
func adminClientSymbols(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(repoRoot, "internal/adapters/driving/grpcapi/interceptors/rbac.go"))
	if err != nil {
		t.Fatalf("read rbac.go: %v", err)
	}
	var out []string
	for _, m := range adminRPCRe.FindAllStringSubmatch(string(b), -1) {
		svc := strings.ToLower(m[1][:1]) + m[1][1:]
		out = append(out, svc+m[2])
	}
	sort.Strings(out)
	return out
}

// webFiles lists the app's own sources, generated code excluded.
func webFiles(t *testing.T) []string {
	t.Helper()
	root := filepath.Join(repoRoot, "frontend/apps/web/src")
	var out []string
	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == "node_modules" || d.Name() == "generated" {
				return filepath.SkipDir
			}
			return nil
		}
		if isClientSource(path) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk web: %v", err)
	}
	sort.Strings(out)
	return out
}

// wrapperHooks finds the app's own exported hooks whose body reaches an admin
// symbol - useDeleteCollection and friends. A page calling one of those reaches
// the admin RPC just as surely as if it had called it by name.
func wrapperHooks(t *testing.T, files, adminSymbols []string) []string {
	t.Helper()
	exportRe := regexp.MustCompile(`export (?:function|const) (use\w+)`)
	var out []string
	for _, f := range files {
		if !strings.Contains(f, "/api.ts") {
			continue
		}
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		if !containsAny(src, adminSymbols) {
			continue
		}
		// Read-only hooks share the file with the writing ones, and the import
		// block at the top names every admin symbol: a hook counts only when
		// its OWN body, from its declaration to the next export, names one.
		locs := exportRe.FindAllStringSubmatchIndex(src, -1)
		for i, loc := range locs {
			end := len(src)
			if i+1 < len(locs) {
				end = locs[i+1][0]
			}
			if containsAny(src[loc[0]:end], adminSymbols) {
				out = append(out, src[loc[2]:loc[3]])
			}
		}
	}
	sort.Strings(out)
	return out
}

func containsAny(src string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(src, n) {
			return true
		}
	}
	return false
}

func classified(rel string) bool {
	for prefix := range gatedByCaller {
		if strings.HasPrefix(rel, prefix) {
			return true
		}
	}
	return false
}

func TestEveryWebSurfaceReachingAnAdminRPCIsClassified(t *testing.T) {
	symbols := adminClientSymbols(t)
	if len(symbols) < 20 {
		t.Fatalf("parsed %d admin RPCs from rbac.go, want at least 20 - the parser has drifted", len(symbols))
	}
	files := webFiles(t)
	if len(files) < 100 {
		t.Fatalf("walked %d web sources, want at least 100 - the walk has drifted", len(files))
	}
	reach := append(append([]string{}, symbols...), wrapperHooks(t, files, symbols)...)

	root := filepath.Join(repoRoot, "frontend/apps/web/src") + "/"
	var ungated []string
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatalf("read %s: %v", f, err)
		}
		src := string(b)
		if !containsAny(src, reach) || strings.Contains(src, "isAdmin") {
			continue
		}
		rel := strings.TrimPrefix(f, root)
		if classified(rel) {
			continue
		}
		ungated = append(ungated, rel)
	}
	if len(ungated) > 0 {
		t.Errorf("web surfaces reach an admin-only RPC without deciding who sees them: %v\n"+
			"Gate the control on isAdmin, or add the file to gatedByCaller naming the surfaces that gate it.", ungated)
	}
}

func TestPinnedSurfacesStillDecideWhoSeesTheAdminControl(t *testing.T) {
	root := filepath.Join(repoRoot, "frontend/apps/web/src")
	for rel, why := range mustGate {
		b, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Errorf("pinned surface %s is unreadable (%v) - if it moved, move the entry too", rel, err)
			continue
		}
		if !strings.Contains(string(b), "isAdmin") {
			t.Errorf("%s no longer decides who sees its admin control: %s", rel, why)
		}
	}
}

// The detector must be seen firing, or a green run proves nothing.
func TestAdminGateGuardFiresOnAnUngatedFixture(t *testing.T) {
	dir := t.TempDir()
	ungated := filepath.Join(dir, "Ungated.tsx")
	if err := os.WriteFile(ungated, []byte("import { collectionServiceDeleteCollection } from '@evey/api';\nexport const X = () => collectionServiceDeleteCollection('c1');\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	gated := filepath.Join(dir, "Gated.tsx")
	if err := os.WriteFile(gated, []byte("import { collectionServiceDeleteCollection } from '@evey/api';\nexport const Y = () => isAdmin && collectionServiceDeleteCollection('c1');\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	symbols := adminClientSymbols(t)

	for _, tc := range []struct {
		path string
		want bool
	}{{ungated, true}, {gated, false}} {
		b, err := os.ReadFile(tc.path)
		if err != nil {
			t.Fatal(err)
		}
		src := string(b)
		got := containsAny(src, symbols) && !strings.Contains(src, "isAdmin")
		if got != tc.want {
			t.Errorf("%s flagged = %v, want %v", filepath.Base(tc.path), got, tc.want)
		}
	}
}
