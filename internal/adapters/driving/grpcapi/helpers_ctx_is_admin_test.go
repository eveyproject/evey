package grpcapi

import (
	"context"
	"testing"
)

// ctxIsAdmin gates admin-only fields (absolute file paths) in the converters:
// no claims and a plain user must not get them, an admin must.
func TestCtxIsAdmin(t *testing.T) {
	if ctxIsAdmin(context.Background()) {
		t.Error("no claims read as admin")
	}
	if ctxIsAdmin(ctxWithClaims("user-1", "profile-1", false)) {
		t.Error("a plain user read as admin")
	}
	if !ctxIsAdmin(ctxWithClaims("user-1", "profile-1", true)) {
		t.Error("an admin did not read as admin: the admin screens lose the file paths")
	}
}
