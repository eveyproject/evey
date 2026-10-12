package interceptors

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/user"
)

// "Connect as" acts as the user for minutes; it must not mint what outlives
// it (an API key, a Quick Connect device), chain another impersonation, or
// change the target's profile PIN. Everything else, and every ordinary
// session, goes through.
func TestImpersonationGuard(t *testing.T) {
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	call := func(claims *user.Claims, method string, req any) error {
		ctx := ContextWithClaims(context.Background(), claims)
		_, err := NewImpersonationGuard(nil).Unary()(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, handler)
		return err
	}
	imp := &user.Claims{UserID: "target", ImpersonatedBy: "admin-1"}
	own := &user.Claims{UserID: "target"}
	pin := "1234"

	for _, m := range []string{"/evey.v1.UserService/CreateAPIKey", "/evey.v1.UserService/AuthorizeQuickConnect", "/evey.v1.UserService/ImpersonateUser"} {
		if st, _ := status.FromError(call(imp, m, nil)); st.Code() != codes.PermissionDenied {
			t.Errorf("%s under impersonation: %v, want PermissionDenied", m, st.Code())
		}
		if err := call(own, m, nil); err != nil {
			t.Errorf("%s in an ordinary session: %v", m, err)
		}
	}
	if st, _ := status.FromError(call(imp, "/evey.v1.UserService/UpdateProfile", &eveyv1.UpdateProfileRequest{Pin: &pin})); st.Code() != codes.PermissionDenied {
		t.Errorf("PIN change under impersonation: %v, want PermissionDenied", st.Code())
	}
	name := "Kids"
	if err := call(imp, "/evey.v1.UserService/UpdateProfile", &eveyv1.UpdateProfileRequest{Name: &name}); err != nil {
		t.Errorf("profile rename under impersonation: %v", err)
	}
	if err := call(imp, "/evey.v1.LibraryService/ListMediaItems", nil); err != nil {
		t.Errorf("browsing under impersonation: %v", err)
	}
}
