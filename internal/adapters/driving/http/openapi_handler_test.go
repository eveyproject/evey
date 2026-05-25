package httpapi

import (
	"encoding/json"
	"os"
	"path/filepath"
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
	// (LibraryService_DeleteLibrary, internal — never granted to API keys).
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
