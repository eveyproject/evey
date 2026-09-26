package contractguard

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// The playback decision is the plan's (PlanResolver.Decide, ADR-0030): the
// pure core, MakePlaybackDecision*, then the request-time pins (the served
// audio track, the burn-in track, the audio resync, the vd=/ad=/vc= pins,
// the HDR tone-map re-add). A production caller reaching the core directly
// decides without those pins - it showed (2026-09-26) as a stream info that
// said "copy" where the plan transcoded the same request.
//
// Two callers remain, listed with what makes them tolerable and what closes
// them; a third is a red test.
var playbackDecisionCoreCallers = map[string]string{
	"internal/adapters/driving/grpcapi/playback_handler.go": "GetStreamInfo, the pre-play info: the client keeps the served variant's truth over it (packages/client streamInfoModel) - to route through Decide once a decider can be bound to the gRPC handler after the HTTP wiring that owns the resolver",
	"internal/adapters/driving/http/transcode_handler.go":   "playback-info, the same pre-play info over HTTP - closes with the gRPC one",
}

func TestOnlyThePlanDecidesPlayback(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	core := map[string]bool{"MakePlaybackDecision": true, "MakePlaybackDecisionWithPrefs": true}
	var violations []string
	seen := map[string]bool{}
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
				if !ok || !core[sel.Sel.Name] {
					return true
				}
				if _, tolerated := playbackDecisionCoreCallers[rel]; tolerated {
					seen[rel] = true
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
		t.Errorf("a caller decides playback on the pure core instead of the plan's Decide (ADR-0030):\n  %s",
			strings.Join(violations, "\n  "))
	}
	for rel := range playbackDecisionCoreCallers {
		if !seen[rel] {
			t.Errorf("%s no longer calls the decision core: drop it from playbackDecisionCoreCallers", rel)
		}
	}
}
