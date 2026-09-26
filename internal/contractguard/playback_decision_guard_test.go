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
// No production caller remains: the two pre-play info surfaces (gRPC
// GetStreamInfo, HTTP playback-info) decide through a PlaybackDecider bound
// to the HLS pipeline's resolver. A tolerated caller would be listed here
// with what makes it tolerable and what closes it.
var playbackDecisionCoreCallers = map[string]string{}

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

// The gRPC handler is built before the HTTP pipeline that owns the resolver,
// so its decider is bound late (GRPCServer.SetPlaybackDecider). Unbound, it
// decides on the unwired resolver: no calibration, no copy producer - a stream
// info that can say "transcode" where the served master copies. The binding
// must happen in main, before the gRPC server serves.
func TestMainBindsThePlaybackDeciderBeforeServing(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, repoRoot+"/cmd/evey/main.go", nil, 0)
	if err != nil {
		t.Fatal(err)
	}
	bound, served := 0, 0
	ast.Inspect(f, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		recv, isIdent := sel.X.(*ast.Ident)
		if !isIdent || recv.Name != "grpcSrv" {
			return true
		}
		switch sel.Sel.Name {
		case "SetPlaybackDecider":
			bound = fset.Position(call.Pos()).Line
		case "Start":
			served = fset.Position(call.Pos()).Line
		}
		return true
	})
	if bound == 0 {
		t.Fatal("cmd/evey/main.go never calls grpcSrv.SetPlaybackDecider: GetStreamInfo decides on the unwired resolver")
	}
	if served == 0 {
		t.Fatal("cmd/evey/main.go never calls grpcSrv.Start")
	}
	if bound > served {
		t.Errorf("cmd/evey/main.go binds the playback decider (line %d) after the gRPC server starts (line %d)", bound, served)
	}
}
