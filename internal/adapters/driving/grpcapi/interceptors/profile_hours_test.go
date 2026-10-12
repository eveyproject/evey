package interceptors

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/genproto/googleapis/rpc/errdetails"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/eveyproject/evey/internal/domain/user"
)

type stubGate struct {
	acc user.ProfileAccess
	err error
}

func (g stubGate) Access(context.Context, string) (user.ProfileAccess, error) { return g.acc, g.err }

// A closed kids profile reaches only the calls it needs to leave; the refusal
// carries the reason and the next opening; unreadable hours refuse too.
func TestProfileHoursInterceptor(t *testing.T) {
	next := time.Date(2026, 10, 5, 16, 0, 0, 0, time.UTC)
	handler := func(context.Context, any) (any, error) { return "ok", nil }
	call := func(gate ProfileAccessGate, claims *user.Claims, method string) error {
		ctx := context.Background()
		if claims != nil {
			ctx = ContextWithClaims(ctx, claims)
		}
		_, err := NewProfileHoursInterceptor(gate, nil).Unary()(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, handler)
		return err
	}
	kid := &user.Claims{UserID: "u1", ProfileID: "kid"}
	closed := stubGate{acc: user.ProfileAccess{Open: false, NextOpen: next}}

	err := call(closed, kid, "/evey.v1.LibraryService/ListMediaItems")
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Fatalf("closed kid browsing: %v, want PermissionDenied", err)
	}
	var info *errdetails.ErrorInfo
	for _, d := range st.Details() {
		if i, ok := d.(*errdetails.ErrorInfo); ok {
			info = i
		}
	}
	if info == nil || info.Reason != ProfileClosedReason || info.Metadata["next_open"] != "2026-10-05T16:00:00Z" || info.Metadata["profile_id"] != "kid" {
		t.Errorf("refusal details %+v, want PROFILE_CLOSED with next_open", info)
	}
	for _, m := range []string{"/evey.v1.UserService/RefreshToken", "/evey.v1.UserService/ListProfiles", "/evey.v1.UserService/Logout", "/evey.v1.UserService/GetCurrentUser"} {
		if err := call(closed, kid, m); err != nil {
			t.Errorf("%s must stay open to leave a closed profile: %v", m, err)
		}
	}
	if err := call(stubGate{acc: user.ProfileAccess{Open: true}}, kid, "/evey.v1.LibraryService/ListMediaItems"); err != nil {
		t.Errorf("open profile: %v", err)
	}
	if err := call(closed, &user.Claims{UserID: "svc"}, "/evey.v1.LibraryService/ListMediaItems"); err != nil {
		t.Errorf("no profile (API key): %v, want through", err)
	}
	if err := call(stubGate{err: errors.New("db down")}, kid, "/evey.v1.LibraryService/ListMediaItems"); status.Code(err) != codes.PermissionDenied {
		t.Errorf("unreadable hours: %v, want refused (fail closed)", err)
	}
}
