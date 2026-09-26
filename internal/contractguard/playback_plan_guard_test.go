// PlaybackPlan boundary guard (ADR-0030, "one plan, one decision"): the
// codec, the backend, the route, the ladder, the class, the fingerprint, the
// producer and the delivery strategy (which engine carries each track) of a
// request are resolved ONCE, by PlanResolver.Resolve, and every consumer
// projects the plan. Before the ADR the same facts were
// derived in four places and kept "in sync" by some sixty mirror comments;
// the ADR removed the mirrors, this guard keeps them from growing back.
//
// Three mechanical checks:
//   - the exported surface of *PlanResolver is exactly the plan-level entry
//     points (Resolve, Decide, IsCopy, ProducerForPlan, QualityPolicy): a new
//     exported sub-derivation is a red test until it is argued for here;
//   - no production file outside internal/app/transcoding calls one of the
//     old sub-derivations by name, whatever the receiver;
//   - the planner's HostCapabilitySnapshot is made by the calibration's
//     publish only: no other production site snapshots a capability row.
package contractguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"
)

const planResolverRoot = repoRoot + "/internal/app/transcoding"

// planResolverSurface is the whole exported method set of *PlanResolver.
var planResolverSurface = map[string]bool{
	"Resolve":         true, // the plan
	"Decide":          true, // the decision alone (on-demand masters, audio sync: no route behind it)
	"IsCopy":          true, // a decision-level fact
	"ProducerForPlan": true, // a projection of plan.Route
	"QualityPolicy":   true, // the policy object, not a derivation
	// The whole route for ONE picture drawn outside a player session (the
	// sync-preview clip, the reduced download): the same resolveRoute the plan
	// runs, for a fixed browser target, with no ladder, pin or fingerprint to
	// project. A plan of its own for a consumer that has no session, not a
	// facet of a session's plan re-derived downstream.
	"ResolvePicture": true,
}

// planSubDerivations are the resolver's former entry points that each
// re-derived one facet of the plan; a caller outside the package that names
// one is deriving downstream of the plan.
var planSubDerivations = []string{
	"EffectiveVideoCodec", "ResolveHW", "HWRouteUsable", "LadderClassFor", "ShapeLadder",
	"PinKey", "ProducerFor", "HWTonemapWanted", "HWDeinterlaceWanted", "HDRPassthroughWanted", "HWOverlayWanted",
	// The delivery strategy's gates (which engine carries the video and the
	// audio) were the HTTP adapter's own until they moved into the plan
	// (PlaybackPlan.Delivery); an adapter naming one again is re-deciding.
	"planHLSVideoEligible", "audioAllCopyEligible", "audioTrimUnlocksCopy", "tryServePlanMaster",
	// The cast receiver's line (?maxKbps) and floor (?abr=floor) are plan
	// inputs (PlaybackRequest.LineKbps / ABRFloor -> PlaybackPlan.Fit); the
	// adapter used to re-pin the request to a transcode and serve that second
	// plan under these names.
	"buildCappedMaster", "buildFloorMaster",
}

func TestPlanResolverExportsOnlyPlanLevelEntryPoints(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var got []string
	err := filepath.WalkDir(planResolverRoot, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Recv == nil || len(fn.Recv.List) != 1 || !fn.Name.IsExported() {
				continue
			}
			if receiverTypeName(fn.Recv.List[0].Type) == "PlanResolver" {
				got = append(got, fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(got)
	var extra []string
	for _, m := range got {
		if !planResolverSurface[m] {
			extra = append(extra, m)
		}
	}
	if len(extra) > 0 {
		t.Errorf("PlanResolver exports sub-derivations beyond the plan-level entry points: %v\n"+
			"A consumer needs plan.Route / plan.Ladder / plan.Fingerprint (Resolve), not a facet derived on its own (ADR-0030). "+
			"If a new plan-level entry point is really needed, argue for it in playback_plan_guard_test.go.", extra)
	}
	for m := range planResolverSurface {
		if !slices.Contains(got, m) {
			t.Errorf("PlanResolver no longer exports %s: update planResolverSurface", m)
		}
	}
}

func TestNoCallerDerivesDownstreamOfThePlaybackPlan(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	names := map[string]bool{}
	for _, n := range planSubDerivations {
		names[n] = true
	}
	var violations []string
	for _, root := range []string{repoRoot + "/internal", repoRoot + "/cmd"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if strings.HasPrefix(path, planResolverRoot) {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			rel, _ := filepath.Rel(repoRoot, path)
			ast.Inspect(f, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok {
					return true
				}
				sel, ok := call.Fun.(*ast.SelectorExpr)
				if !ok || !names[sel.Sel.Name] {
					return true
				}
				violations = append(violations, fmt.Sprintf("%s:%d calls %s", rel, fset.Position(call.Pos()).Line, sel.Sel.Name))
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("a caller derives a facet of the playback plan on its own (ADR-0030: resolve the plan, project it):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// snapshotMakers are the production sites allowed to turn a capability row
// into the planner's HostCapabilitySnapshot: the calibration service's
// publish (every store goes through it) and the two SafeFallback stand-ins
// for a host with no calibration service at all. Anywhere else, a row
// reaching the planner bypassed the publish - the type exists to make that
// a red test.
var snapshotMakers = map[string]bool{
	"internal/app/transcoding/calibration_service.go":      true,
	"internal/app/transcoding/resolver_route.go":           true,
	"internal/adapters/driving/http/transcode_vod_wire.go": true,
}

func TestOnlyTheCalibrationPublishesACapabilitySnapshot(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	var violations []string
	for _, root := range []string{repoRoot + "/internal", repoRoot + "/cmd"} {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			rel, _ := filepath.Rel(repoRoot, path)
			if snapshotMakers[rel] || rel == "internal/domain/transcoding/capability.go" {
				return nil
			}
			f, err := parser.ParseFile(fset, path, nil, 0)
			if err != nil {
				return err
			}
			ast.Inspect(f, func(n ast.Node) bool {
				switch x := n.(type) {
				case *ast.CallExpr:
					if sel, ok := x.Fun.(*ast.SelectorExpr); ok && sel.Sel.Name == "Snapshot" && len(x.Args) == 0 && isCapabilityExpr(sel.X) {
						violations = append(violations, fmt.Sprintf("%s:%d calls Snapshot() on a capability row", rel, fset.Position(x.Pos()).Line))
					}
				case *ast.CompositeLit:
					if typeName(x.Type) == "HostCapabilitySnapshot" {
						violations = append(violations, fmt.Sprintf("%s:%d builds a HostCapabilitySnapshot literal", rel, fset.Position(x.Pos()).Line))
					}
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if len(violations) > 0 {
		sort.Strings(violations)
		t.Errorf("a capability snapshot is made outside the calibration's publish (ADR-0030: the planner reads what the calibration published):\n  %s",
			strings.Join(violations, "\n  "))
	}
}

// isCapabilityExpr tells a capability row's Snapshot() from the unrelated
// Snapshot() of other types (the background-task registry): the receiver is
// a call returning a capability (SafeFallback(), Current()) or a selector
// naming one (cap, capability, persisted).
func isCapabilityExpr(x ast.Expr) bool {
	switch e := x.(type) {
	case *ast.CallExpr:
		if sel, ok := e.Fun.(*ast.SelectorExpr); ok {
			return sel.Sel.Name == "SafeFallback" || sel.Sel.Name == "Current"
		}
		if id, ok := e.Fun.(*ast.Ident); ok {
			return id.Name == "SafeFallback"
		}
	case *ast.Ident:
		switch e.Name {
		case "cap", "capability", "persisted", "row", "c":
			return true
		}
	case *ast.SelectorExpr:
		return isCapabilityExpr(e.Sel)
	}
	return false
}

func typeName(expr ast.Expr) string {
	switch e := expr.(type) {
	case *ast.Ident:
		return e.Name
	case *ast.SelectorExpr:
		return e.Sel.Name
	}
	return ""
}

func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
