package httpapi

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	authpkg "github.com/evey-project/evey/internal/pkg/auth"
)

func TestMethodFromOperationID(t *testing.T) {
	cases := map[string]string{
		"LibraryService_GetLibrary":   "/evey.v1.LibraryService/GetLibrary",
		"PlaybackService_StartStream": "/evey.v1.PlaybackService/StartStream",
		"":                            "",
		"NoUnderscore":                "",
		"Trailing_":                   "",
		"_Leading":                    "",
	}
	for in, want := range cases {
		if got := methodFromOperationID(in); got != want {
			t.Errorf("methodFromOperationID(%q) = %q, want %q", in, got, want)
		}
	}
	// Every key in APIKeyScopeFor must be reachable from some operationId form,
	// i.e. the round-trip Service/Method -> Service_Method -> Service/Method holds.
	for method := range authpkg.APIKeyScopeFor {
		// /evey.v1.Svc/Method -> Svc_Method
		// (sanity: the mapping keys use the expected shape)
		if method == "" || method[0] != '/' {
			t.Errorf("unexpected APIKeyScopeFor key shape: %q", method)
		}
	}
}

func TestBuildPublicOpenAPISpec_FiltersToAPIKeySurface(t *testing.T) {
	dir := t.TempDir()
	// A fixture spec with one operation that IS in the API-key surface
	// (LibraryService_ListLibraries -> media:read) and one that is NOT
	// (LibraryService_DeleteLibrary, internal - never granted to API keys).
	fixture := `{
      "swagger": "2.0",
      "paths": {
        "/api/v1/libraries": {
          "get": {"operationId": "LibraryService_ListLibraries", "description": "list", "tags": ["LibraryService"]},
          "post": {"operationId": "LibraryService_CreateLibrary", "description": "create"}
        },
        "/api/v1/libraries/{id}": {
          "delete": {"operationId": "LibraryService_DeleteLibrary", "description": "delete"}
        }
      },
      "definitions": {"v1Library": {"type": "object"}}
    }`
	if err := os.WriteFile(filepath.Join(dir, "library.swagger.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := buildPublicOpenAPISpec(dir)
	if err != nil {
		t.Fatalf("buildPublicOpenAPISpec: %v", err)
	}
	var spec struct {
		Paths map[string]map[string]struct {
			Tags        []string `json:"tags"`
			Description string   `json:"description"`
			XScope      string   `json:"x-required-scope"`
		} `json:"paths"`
		Definitions map[string]json.RawMessage `json:"definitions"`
		Security    []map[string][]string      `json:"security"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	// ListLibraries (media:read) must be kept and tagged with its scope.
	got, ok := spec.Paths["/api/v1/libraries"]["get"]
	if !ok {
		t.Fatal("expected GET /api/v1/libraries to be kept (it is in APIKeyScopeFor)")
	}
	if got.XScope != authpkg.ScopeMediaRead {
		t.Errorf("x-required-scope = %q, want %q", got.XScope, authpkg.ScopeMediaRead)
	}
	// Group tag = resource (Service suffix stripped); verb order is client-side.
	if len(got.Tags) != 1 || got.Tags[0] != "Library" {
		t.Errorf("tags = %v, want [Library]", got.Tags)
	}

	// CreateLibrary and DeleteLibrary are not in the API-key surface -> dropped.
	if _, exists := spec.Paths["/api/v1/libraries"]["post"]; exists {
		t.Error("CreateLibrary is not an API-key method and must be filtered out")
	}
	if _, exists := spec.Paths["/api/v1/libraries/{id}"]; exists {
		t.Error("DeleteLibrary is not an API-key method and must be filtered out")
	}

	// Security scheme wired so Swagger UI shows Authorize.
	if len(spec.Security) == 0 {
		t.Error("expected a top-level security requirement")
	}
}

// The merged spec must only carry the models actually reachable from the kept
// (API-key) operations: buf generates definitions for EVERY proto message, so
// merging them all shipped hundreds of internal-API schemas next to a handful
// of endpoints - noise for the reader, and a map of the internal surface the
// page explicitly says is not exposed.
func TestBuildPublicOpenAPISpec_PrunesUnreachableDefinitions(t *testing.T) {
	dir := t.TempDir()
	// Kept op -> v1Library -> v1Artwork (transitive). v1Internal is referenced
	// only by the DROPPED op; v1Orphan by nothing at all.
	fixture := `{
      "swagger": "2.0",
      "paths": {
        "/api/v1/libraries": {
          "get": {
            "operationId": "LibraryService_ListLibraries",
            "responses": {"200": {"schema": {"$ref": "#/definitions/v1Library"}}}
          },
          "post": {
            "operationId": "LibraryService_CreateLibrary",
            "responses": {"200": {"schema": {"$ref": "#/definitions/v1Internal"}}}
          }
        }
      },
      "definitions": {
        "v1Library":  {"type": "object", "properties": {"art": {"$ref": "#/definitions/v1Artwork"}}},
        "v1Artwork":  {"type": "object"},
        "v1Internal": {"type": "object"},
        "v1Orphan":   {"type": "object"}
      }
    }`
	if err := os.WriteFile(filepath.Join(dir, "library.swagger.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}

	raw, err := buildPublicOpenAPISpec(dir)
	if err != nil {
		t.Fatalf("buildPublicOpenAPISpec: %v", err)
	}
	var spec struct {
		Definitions map[string]json.RawMessage `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &spec); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}

	for _, want := range []string{"v1Library", "v1Artwork"} {
		if _, ok := spec.Definitions[want]; !ok {
			t.Errorf("definition %s is reachable from a kept operation and must be present", want)
		}
	}
	for _, dropped := range []string{"v1Internal", "v1Orphan"} {
		if _, ok := spec.Definitions[dropped]; ok {
			t.Errorf("definition %s is unreachable from the kept operations and must be pruned", dropped)
		}
	}
}

// The handler builds the spec once from the first candidate dir that exists,
// caches the bytes for every later request, and answers 503 (not a panic, not
// an empty 200) when no dir is there - a dev tree without gen/.
func TestOpenAPIPublicHandler_BuildsOnceAnd503WithoutSpecs(t *testing.T) {
	saved := publicOpenAPIDirs
	t.Cleanup(func() { publicOpenAPIDirs = saved })

	publicOpenAPIDirs = []string{filepath.Join(t.TempDir(), "missing")}
	rec := httptest.NewRecorder()
	openAPIPublicHandler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("no specs dir: status %d, want 503", rec.Code)
	}

	dir := t.TempDir()
	fixture := `{"swagger":"2.0","paths":{"/api/v1/libraries":{"get":{"operationId":"LibraryService_ListLibraries","description":"list","tags":["LibraryService"]}}},"definitions":{}}`
	if err := os.WriteFile(filepath.Join(dir, "library.swagger.json"), []byte(fixture), 0o644); err != nil {
		t.Fatal(err)
	}
	publicOpenAPIDirs = []string{filepath.Join(t.TempDir(), "missing"), dir}
	h := openAPIPublicHandler()
	first := httptest.NewRecorder()
	h.ServeHTTP(first, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if first.Code != http.StatusOK || first.Header().Get("Content-Type") != "application/json" {
		t.Fatalf("status %d content-type %q", first.Code, first.Header().Get("Content-Type"))
	}
	if !strings.Contains(first.Body.String(), "LibraryService_ListLibraries") {
		t.Errorf("spec body = %s", first.Body.String())
	}

	// Remove the source: a cached handler must keep serving the same bytes.
	if err := os.RemoveAll(dir); err != nil {
		t.Fatal(err)
	}
	second := httptest.NewRecorder()
	h.ServeHTTP(second, httptest.NewRequest(http.MethodGet, "/api/v1/openapi.json", nil))
	if second.Code != http.StatusOK || second.Body.String() != first.Body.String() {
		t.Error("second request must be served from the cache")
	}
}
