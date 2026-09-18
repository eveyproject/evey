// PlaybackPlan boundary guard (ADR-0030, "one plan, one decision"): the
// codec, the backend, the route, the ladder, the class, the fingerprint and
// the producer of a request are resolved ONCE, by PlanResolver.Resolve, and
// every consumer projects the plan. Before the ADR the same facts were
// derived in four places and kept "in sync" by some sixty mirror comments;
// the ADR removed the mirrors, this guard keeps them from growing back.
//
// Two mechanical checks:
//   - the exported surface of *PlanResolver is exactly the plan-level entry
//     points (Resolve, Decide, IsCopy, ProducerForPlan, QualityPolicy): a new
//     exported sub-derivation is a red test until it is argued for here;
//   - no production file outside internal/app/transcoding calls one of the
//     old sub-derivations by name (they live on as test-only wrappers inside
//     the package), whatever the receiver.
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
}

// planSubDerivations are the resolver's former entry points that each
// re-derived one facet of the plan; a caller outside the package that names
// one is deriving downstream of the plan.
var planSubDerivations = []string{
	"EffectiveVideoCodec", "ResolveHW", "HWRouteUsable", "LadderClassFor", "ShapeLadder",
	"PinKey", "ProducerFor", "HWTonemapWanted", "HWDeinterlaceWanted", "HDRPassthroughWanted", "HWOverlayWanted",
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

func receiverTypeName(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if id, ok := expr.(*ast.Ident); ok {
		return id.Name
	}
	return ""
}
