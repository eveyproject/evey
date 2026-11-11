package grpcapi

import (
	"context"
	"maps"
	"sort"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// accountAudit says how one account or profile RPC lands in the audit log.
type accountAudit struct {
	action       string
	resourceType string
	// resourceID names what the call acted on, from the request or, for a
	// creation, the response (nil when the call failed before creating it).
	resourceID func(req, resp any) string
	// details adds what a success may say beyond the field names (never a
	// secret, never a value the caller typed into a refused request).
	details func(resp any) map[string]any
}

const userSvcPrefix = "/evey.v1.UserService/"

func reqID[T interface{ GetId() string }](req, _ any) string {
	if r, ok := req.(T); ok {
		return r.GetId()
	}
	return ""
}

func respID[T interface{ GetId() string }](_, resp any) string {
	if r, ok := resp.(T); ok {
		return r.GetId()
	}
	return ""
}

func authUserID(_, resp any) string {
	if r, ok := resp.(*eveyv1.AuthResponse); ok {
		return r.GetUser().GetId()
	}
	return ""
}

func impersonatedID(req, _ any) string {
	if r, ok := req.(*eveyv1.ImpersonateUserRequest); ok {
		return r.GetUserId()
	}
	return ""
}

func userSummary(resp any) map[string]any {
	if u, ok := resp.(*eveyv1.User); ok {
		return map[string]any{"username": u.GetUsername(), "is_admin": u.GetIsAdmin()}
	}
	if a, ok := resp.(*eveyv1.AuthResponse); ok && a.GetUser() != nil {
		return map[string]any{"username": a.GetUser().GetUsername()}
	}
	return nil
}

// accountAudits lists every RPC that changes an account, a profile or a way
// in (invite, API key, quick connect, password reset). Login and Logout audit
// themselves in their handlers.
var accountAudits = map[string]accountAudit{
	userSvcPrefix + "CreateUser":            {action: "create_user", resourceType: "user", resourceID: respID[*eveyv1.User], details: userSummary},
	userSvcPrefix + "UpdateUser":            {action: "update_user", resourceType: "user", resourceID: reqID[*eveyv1.UpdateUserRequest]},
	userSvcPrefix + "DeleteUser":            {action: "delete_user", resourceType: "user", resourceID: reqID[*eveyv1.DeleteUserRequest]},
	userSvcPrefix + "DisableUser":           {action: "disable_user", resourceType: "user", resourceID: reqID[*eveyv1.DisableUserRequest]},
	userSvcPrefix + "EnableUser":            {action: "enable_user", resourceType: "user", resourceID: reqID[*eveyv1.EnableUserRequest]},
	userSvcPrefix + "ImpersonateUser":       {action: "impersonate_user", resourceType: "user", resourceID: impersonatedID},
	userSvcPrefix + "CreateProfile":         {action: "create_profile", resourceType: "profile", resourceID: respID[*eveyv1.Profile]},
	userSvcPrefix + "UpdateProfile":         {action: "update_profile", resourceType: "profile", resourceID: reqID[*eveyv1.UpdateProfileRequest]},
	userSvcPrefix + "DeleteProfile":         {action: "delete_profile", resourceType: "profile", resourceID: reqID[*eveyv1.DeleteProfileRequest]},
	userSvcPrefix + "SetDefaultProfile":     {action: "set_default_profile", resourceType: "profile", resourceID: reqID[*eveyv1.SetDefaultProfileRequest]},
	userSvcPrefix + "CreateInviteLink":      {action: "create_invite_link", resourceType: "invite_link", resourceID: respID[*eveyv1.InviteLink]},
	userSvcPrefix + "RevokeInviteLink":      {action: "revoke_invite_link", resourceType: "invite_link", resourceID: reqID[*eveyv1.RevokeInviteLinkRequest]},
	userSvcPrefix + "RegisterWithInvite":    {action: "register_with_invite", resourceType: "user", resourceID: authUserID, details: userSummary},
	userSvcPrefix + "AuthorizeQuickConnect": {action: "authorize_quick_connect", resourceType: "user"},
	userSvcPrefix + "CreateAPIKey":          {action: "create_api_key", resourceType: "api_key", resourceID: respID[*eveyv1.APIKey]},
	userSvcPrefix + "RevokeAPIKey":          {action: "revoke_api_key", resourceType: "api_key", resourceID: reqID[*eveyv1.RevokeAPIKeyRequest]},
	userSvcPrefix + "RequestPasswordReset":  {action: "request_password_reset", resourceType: "user"},
	userSvcPrefix + "ResetPassword":         {action: "reset_password", resourceType: "user"},
}

// AccountAuditUnary records every account and profile change in the audit
// log, the refused ones too: who asked, on what, which fields the request
// carried (names only, never values) and, when refused, the gRPC code and the
// message key. Placed after auth, so the caller's claims are on ctx.
func AccountAuditUnary(svc *appactivity.CommandService) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		spec, ok := accountAudits[info.FullMethod]
		if !ok || svc == nil {
			return handler(ctx, req)
		}
		resp, err := handler(ctx, req)
		recordAccountAudit(ctx, svc, spec, req, resp, err)
		return resp, err
	}
}

func recordAccountAudit(ctx context.Context, svc *appactivity.CommandService, spec accountAudit, req, resp any, err error) {
	details := map[string]any{}
	if fields := requestFields(req); len(fields) > 0 {
		details["fields"] = fields
	}
	if err != nil {
		details["outcome"] = "refused"
		details["code"] = status.Code(err).String()
		if key := eveyi18n.ErrorKey(err); key != "" {
			details["reason"] = key
		}
	} else {
		details["outcome"] = "success"
		if spec.details != nil {
			maps.Copy(details, spec.details(resp))
		}
	}
	var resourceID *string
	if spec.resourceID != nil {
		var r any
		if err == nil {
			r = resp
		}
		if id := spec.resourceID(req, r); id != "" {
			resourceID = &id
		}
	}
	actor := ""
	if c := interceptors.ClaimsFromContext(ctx); c != nil {
		actor = c.UserID
	}
	recordAudit(ctx, svc, actor, spec.action, spec.resourceType, resourceID, details)
}

// requestFields names the fields the request set, sorted, ids left out: a
// password, PIN or e-mail shows as its field name only.
func requestFields(req any) []string {
	m, ok := req.(proto.Message)
	if !ok {
		return nil
	}
	var names []string
	m.ProtoReflect().Range(func(fd protoreflect.FieldDescriptor, _ protoreflect.Value) bool {
		switch n := string(fd.Name()); n {
		case "id", "user_id":
		default:
			names = append(names, n)
		}
		return true
	})
	sort.Strings(names)
	return names
}
