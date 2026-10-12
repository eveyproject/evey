package interceptors

import (
	"context"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
)

func extendedCall(t *testing.T, d *DemoInterceptor, method string, req any) error {
	t.Helper()
	_, err := d.Unary()(context.Background(), req, &grpc.UnaryServerInfo{FullMethod: method},
		func(context.Context, any) (any, error) { return nil, nil })
	return err
}

func TestExtendedDemoProtectsTheReviewersOwnAccount(t *testing.T) {
	d := NewDemoInterceptor(DefaultRBACConfig(), nil).
		Extended(func(context.Context) string { return "own-id" }, "/data/demo-media")

	for _, method := range []string{
		"/evey.v1.UserService/UpdateUser", "/evey.v1.UserService/DisableUser", "/evey.v1.UserService/DeleteUser",
	} {
		var own, other any
		switch method {
		case "/evey.v1.UserService/UpdateUser":
			own, other = &eveyv1.UpdateUserRequest{Id: "own-id"}, &eveyv1.UpdateUserRequest{Id: "someone-else"}
		case "/evey.v1.UserService/DisableUser":
			own, other = &eveyv1.DisableUserRequest{Id: "own-id"}, &eveyv1.DisableUserRequest{Id: "someone-else"}
		default:
			own, other = &eveyv1.DeleteUserRequest{Id: "own-id"}, &eveyv1.DeleteUserRequest{Id: "someone-else"}
		}
		if got := status.Code(extendedCall(t, d, method, own)); got != codes.PermissionDenied {
			t.Errorf("%s on the guest's own account = %v, want PermissionDenied", method, got)
		}
		if err := extendedCall(t, d, method, other); err != nil {
			t.Errorf("%s on another account must stay open on extended, got %v", method, err)
		}
	}
}

func TestExtendedDemoKeepsLibrariesInsideTheDemoMediaDir(t *testing.T) {
	d := NewDemoInterceptor(DefaultRBACConfig(), nil).
		Extended(func(context.Context) string { return "" }, "/data/demo-media")

	for path, ok := range map[string]bool{
		"/data/demo-media":             true,
		"/data/demo-media/films":       true,
		"/data/demo-media/../postgres": false,
		"/data/demo-media-evil":        false,
		"/etc":                         false,
		"/":                            false,
		"films":                        false,
		"":                             false,
	} {
		for name, req := range map[string]any{
			"create": &eveyv1.CreateLibraryRequest{Name: "x", Paths: []string{path}},
			"update": &eveyv1.UpdateLibraryRequest{Id: "l", Paths: []string{path}},
		} {
			method := "/evey.v1.LibraryService/CreateLibrary"
			if name == "update" {
				method = "/evey.v1.LibraryService/UpdateLibrary"
			}
			err := extendedCall(t, d, method, req)
			if ok && err != nil {
				t.Errorf("%s with %q should pass, got %v", name, path, err)
			}
			if !ok && status.Code(err) != codes.PermissionDenied {
				t.Errorf("%s with %q should be refused, got %v", name, path, err)
			}
		}
	}
}

func TestPublicDemoIsUnchangedByTheExtendedChecks(t *testing.T) {
	d := NewDemoInterceptor(DefaultRBACConfig(), nil)
	for _, method := range []string{"/evey.v1.UserService/UpdateUser", "/evey.v1.LibraryService/CreateLibrary"} {
		if got := status.Code(extendedCall(t, d, method, &eveyv1.UpdateUserRequest{Id: "x"})); got != codes.PermissionDenied {
			t.Errorf("%s must stay locked on the public demo, got %v", method, got)
		}
	}
}
