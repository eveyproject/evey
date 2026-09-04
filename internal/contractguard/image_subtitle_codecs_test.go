package contractguard

// The server and the players must agree on which subtitle codecs are BITMAPS.
//
// The disagreement is silent and total. A bitmap track the client mistakes for
// text is offered in the picker like any other; the player asks for a WebVTT
// sidecar; the server answers 422 because pictures cannot be turned into text;
// nothing is drawn and nothing is said. The viewer sees a subtitle track that
// does nothing when selected.
//
// That is not hypothetical: until 2026-09-04 the three players carried their own
// copy-pasted list, and all three were missing "dvb_subtitle" - the CANONICAL
// name ffprobe reports and the prober stores - and "dvdsub". Every DVB bitmap
// subtitle in the library was a dead entry in the menu.
//
// The lists now live in one place each, and this fails the build if they drift:
//
//	Go: internal/domain/transcoding/subtitle_codecs.go (IsImageSubtitle)
//	TS: frontend/packages/api/src/imageSubtitles.ts   (IMAGE_SUBTITLE_CODECS)
//
// The Go side is read with go/ast rather than grepped, because a switch body
// survives reformatting but a regex over it does not.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// goImageSubtitleCodecs returns every string case of the IsImageSubtitle switch.
func goImageSubtitleCodecs(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot, "internal/domain/transcoding/subtitle_codecs.go")
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	var out []string
	var found bool
	ast.Inspect(f, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != "IsImageSubtitle" {
			return true
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			cc, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, e := range cc.List {
				if s := stringLit(e); s != "" {
					out = append(out, s)
				}
			}
			return true
		})
		return false
	})
	if !found {
		t.Fatalf("%s: IsImageSubtitle not found - was it renamed? this guard must be updated with it", path)
	}
	return out
}

// tsStringLiteral matches the quoted entries of the TS Set literal.
var tsStringLiteral = regexp.MustCompile(`"([a-z0-9_]+)"`)

// tsImageSubtitleCodecs returns the entries of the IMAGE_SUBTITLE_CODECS set.
// A regex is honest here: the file is not Go, and the literal it reads is a flat
// list of quoted lowercase names with no nesting to get wrong. The Set body is
// isolated first so the surrounding prose - which names these very codecs - is
// not scooped up with it.
func tsImageSubtitleCodecs(t *testing.T) []string {
	t.Helper()
	path := filepath.Join(repoRoot, "frontend/packages/api/src/imageSubtitles.ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	src := string(raw)

	const marker = "IMAGE_SUBTITLE_CODECS: ReadonlySet<string> = new Set(["
	start := strings.Index(src, marker)
	if start < 0 {
		t.Fatalf("%s: IMAGE_SUBTITLE_CODECS set literal not found - was it renamed or reshaped? "+
			"this guard must be updated with it", path)
	}
	body := src[start+len(marker):]
	end := strings.Index(body, "])")
	if end < 0 {
		t.Fatalf("%s: unterminated IMAGE_SUBTITLE_CODECS literal", path)
	}

	var out []string
	for _, m := range tsStringLiteral.FindAllStringSubmatch(body[:end], -1) {
		out = append(out, m[1])
	}
	return out
}

func TestImageSubtitleCodecsAgreeAcrossServerAndClients(t *testing.T) {
	goSet := goImageSubtitleCodecs(t)
	tsSet := tsImageSubtitleCodecs(t)

	if len(goSet) == 0 || len(tsSet) == 0 {
		t.Fatalf("parsed %d Go codecs and %d TS codecs - the guard would pass vacuously", len(goSet), len(tsSet))
	}

	inGo := make(map[string]bool, len(goSet))
	for _, c := range goSet {
		inGo[c] = true
	}
	inTS := make(map[string]bool, len(tsSet))
	for _, c := range tsSet {
		inTS[c] = true
	}

	var missingInTS, missingInGo []string
	for c := range inGo {
		if !inTS[c] {
			missingInTS = append(missingInTS, c)
		}
	}
	for c := range inTS {
		if !inGo[c] {
			missingInGo = append(missingInGo, c)
		}
	}
	sort.Strings(missingInTS)
	sort.Strings(missingInGo)

	if len(missingInTS) > 0 {
		t.Errorf("the server treats %v as bitmap subtitles but the players do not "+
			"(frontend/packages/api/src/imageSubtitles.ts): those tracks are offered as text, the player "+
			"requests a sidecar, the server answers 422, and selecting them does nothing", missingInTS)
	}
	if len(missingInGo) > 0 {
		t.Errorf("the players treat %v as bitmap subtitles but the server does not "+
			"(internal/domain/transcoding/subtitle_codecs.go): those tracks would be requested as burn-in "+
			"and the server would never burn them, so selecting them does nothing", missingInGo)
	}
}

// The copies are gone; nothing may quietly grow a new one.
func TestNoPlayerRedeclaresTheImageSubtitleCodecList(t *testing.T) {
	roots := []string{
		filepath.Join(repoRoot, "frontend/apps/web/src"),
		filepath.Join(repoRoot, "frontend/apps/mobile/app"),
		filepath.Join(repoRoot, "frontend/apps/mobile/src"),
		filepath.Join(repoRoot, "frontend/apps/tv/app"),
		filepath.Join(repoRoot, "frontend/apps/tv/src"),
	}
	// A local declaration, not a use: importing the shared one is the point.
	decl := regexp.MustCompile(`(const|let|var)\s+IMAGE_SUBTITLE_CODECS\s*[:=]`)

	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() {
				return err //nolint:wrapcheck // walk error is propagated verbatim on purpose
			}
			if ext := filepath.Ext(path); ext != ".ts" && ext != ".tsx" {
				return nil
			}
			raw, readErr := os.ReadFile(path) //nolint:gosec // path comes from walking the repo
			if readErr != nil {
				return readErr //nolint:wrapcheck // propagated verbatim
			}
			if decl.Match(raw) {
				t.Errorf("%s declares its own IMAGE_SUBTITLE_CODECS: import { isImageSubtitle } from \"@evey/api\" "+
					"instead. Three private copies had already drifted from the server once, and every bitmap "+
					"subtitle they missed became a menu entry that does nothing", path)
			}
			return nil
		})
		if err != nil && !os.IsNotExist(err) {
			t.Fatalf("walk %s: %v", root, err)
		}
	}
}
