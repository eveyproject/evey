package httpapi

import (
	"os"
	"strings"
	"testing"

	apploadtest "github.com/eveyproject/evey/internal/app/loadtest"
)

// The route is spelled out for the route guards, which read the source; the
// run asks for it by apploadtest.LoadTestPeerPath. The two must stay one path.
func TestLoadTestPeerRouteIsTheOneTheRunAsks(t *testing.T) {
	src, err := os.ReadFile("server.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), `"GET `+apploadtest.LoadTestPeerPath+`", admin(apploadtest.PeerHandler)`) {
		t.Fatalf("server.go does not register %s behind the admin middleware", apploadtest.LoadTestPeerPath)
	}
}
