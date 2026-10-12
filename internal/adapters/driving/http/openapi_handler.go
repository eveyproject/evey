package httpapi

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"

	authpkg "github.com/eveyproject/evey/internal/pkg/auth"
)

// publicOpenAPIDirs are the candidate locations of the buf-generated OpenAPI v2
// specs, tried in order. gen/ is copied into the prod image at /app/gen and the
// repo is mounted at /app in dev, so a workdir-relative path covers both; the
// absolute path is a fallback for other launch dirs.
var publicOpenAPIDirs = []string{"gen/evey/v1", "/app/gen/evey/v1"}

// methodFromOperationID turns a buf operationId ("LibraryService_GetLibrary")
// into the gRPC full method name APIKeyScopeFor is keyed by
// ("/evey.v1.LibraryService/GetLibrary"). Returns "" when it does not look like
// "Service_Method" (proto service/method names never contain underscores, so
// splitting on the first one is unambiguous).
func methodFromOperationID(opID string) string {
	i := strings.IndexByte(opID, '_')
	if i <= 0 || i == len(opID)-1 {
		return ""
	}
	return "/evey.v1." + opID[:i] + "/" + opID[i+1:]
}

// buildPublicOpenAPISpec aggregates the per-service OpenAPI v2 specs in dir and
// keeps only the operations callable with an API key (those mapped in
// APIKeyScopeFor), tagging each with its required scope. The result is a single
// Swagger 2.0 document describing exactly the public (API-key) surface - it
// stays in sync with the code because both the specs (buf) and APIKeyScopeFor
// are generated/maintained alongside the proto.
func buildPublicOpenAPISpec(dir string) ([]byte, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	mergedPaths := map[string]map[string]any{}
	mergedDefs := map[string]json.RawMessage{}
	kept := 0

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".swagger.json") {
			continue
		}
		raw, readErr := os.ReadFile(filepath.Join(dir, e.Name()))
		if readErr != nil {
			return nil, readErr
		}
		var doc struct {
			Paths       map[string]map[string]json.RawMessage `json:"paths"`
			Definitions map[string]json.RawMessage            `json:"definitions"`
		}
		if uErr := json.Unmarshal(raw, &doc); uErr != nil {
			return nil, fmt.Errorf("%s: %w", e.Name(), uErr)
		}
		// Collect every definition for now; only those reachable from the kept
		// operations survive below - buf generates one schema per proto message
		// across the WHOLE API, which is hundreds of internal models this page
		// must neither drown the reader in nor disclose.
		maps.Copy(mergedDefs, doc.Definitions)
		for path, verbs := range doc.Paths {
			for verb, opRaw := range verbs {
				var op map[string]any
				if opErr := json.Unmarshal(opRaw, &op); opErr != nil {
					continue
				}
				opID, _ := op["operationId"].(string)
				scope, ok := authpkg.APIKeyScopeFor[methodFromOperationID(opID)]
				if !ok {
					continue // internal method: not part of the API-key surface
				}
				// Keep buf's original tag (the service) so Swagger UI groups by
				// resource - all of a resource's operations together - rather than
				// splitting them across scopes. Surface the scope in the
				// description and as an extension instead.
				desc, _ := op["description"].(string)
				op["description"] = "**Required scope: `" + scope + "`**\n\n" + desc
				op["x-required-scope"] = scope
				// Group by resource: strip the "Service" suffix from buf's tag.
				// Verb ordering within the group is done client-side.
				if tags, ok := op["tags"].([]any); ok {
					for i, tg := range tags {
						if s, ok := tg.(string); ok {
							tags[i] = strings.TrimSuffix(s, "Service")
						}
					}
				}
				if mergedPaths[path] == nil {
					mergedPaths[path] = map[string]any{}
				}
				mergedPaths[path][verb] = op
				kept++
			}
		}
	}

	defs, err := reachableDefinitions(mergedPaths, mergedDefs)
	if err != nil {
		return nil, err
	}

	spec := map[string]any{
		"swagger": "2.0",
		"info": map[string]any{
			"title":       "Evey Public API",
			"version":     "v1",
			"description": fmt.Sprintf("The %d endpoints callable with an API key, grouped by resource; each operation lists the scope it requires. Authorize with your key as a Bearer token (`Bearer evey_…`). Everything else in the server is internal and not reachable with an API key.", kept),
		},
		"consumes":    []string{"application/json"},
		"produces":    []string{"application/json"},
		"paths":       mergedPaths,
		"definitions": defs,
		"securityDefinitions": map[string]any{
			"ApiKeyAuth": map[string]any{
				"type":        "apiKey",
				"name":        "Authorization",
				"in":          "header",
				"description": "API key as a Bearer token: `Bearer evey_…`",
			},
		},
		"security": []map[string]any{{"ApiKeyAuth": []string{}}},
	}
	return json.Marshal(spec)
}

// defRefRe matches "#/definitions/<name>" references inside raw OpenAPI v2
// JSON - resilient to where the $ref sits (response schema, body parameter,
// nested property, array items).
var defRefRe = regexp.MustCompile(`#/definitions/([A-Za-z0-9_.]+)`)

// reachableDefinitions keeps only the transitive closure of the definitions
// referenced by the kept operations: seed with the $refs found in the
// operations themselves, then follow refs between definitions.
func reachableDefinitions(paths map[string]map[string]any, defs map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	opsRaw, err := json.Marshal(paths)
	if err != nil {
		return nil, fmt.Errorf("marshal kept operations: %w", err)
	}
	seen := map[string]bool{}
	var queue []string
	push := func(raw []byte) {
		for _, m := range defRefRe.FindAllSubmatch(raw, -1) {
			if name := string(m[1]); !seen[name] {
				seen[name] = true
				queue = append(queue, name)
			}
		}
	}
	push(opsRaw)
	out := map[string]json.RawMessage{}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		def, ok := defs[name]
		if !ok {
			continue // dangling ref: leave it dangling rather than fail the page
		}
		out[name] = def
		push(def)
	}
	return out, nil
}

// openAPIPublicHandler serves the filtered public API spec as JSON. The spec is
// built once and cached (a proto change requires a server restart anyway, which
// regenerates gen/). Returns 503 if the generated specs are not found.
func openAPIPublicHandler() http.HandlerFunc {
	var (
		once   sync.Once
		cached []byte
		cErr   error
	)
	build := func() {
		for _, dir := range publicOpenAPIDirs {
			if _, err := os.Stat(dir); err == nil {
				cached, cErr = buildPublicOpenAPISpec(dir)
				if cErr == nil {
					slog.Info("openapi: public spec built", slog.String("dir", dir), slog.Int("bytes", len(cached)))
				}
				return
			}
		}
		cErr = fmt.Errorf("openapi: specs dir not found in %v", publicOpenAPIDirs)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		once.Do(build)
		if cErr != nil {
			slog.ErrorContext(r.Context(), "openapi: build public spec", slog.Any("err", cErr))
			http.Error(w, `{"message":"openapi spec unavailable"}`, http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "public, max-age=300")
		_, _ = w.Write(cached)
	}
}
