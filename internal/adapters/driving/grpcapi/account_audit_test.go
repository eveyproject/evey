package grpcapi

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	appactivity "github.com/eveyproject/evey/internal/app/activity"
	"github.com/eveyproject/evey/internal/domain/activity"
	activityfakes "github.com/eveyproject/evey/internal/domain/activity/fakes"
	sharedfakes "github.com/eveyproject/evey/internal/domain/shared/fakes"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

func runAccountAudit(t *testing.T, method string, req any, handler grpc.UnaryHandler) *activityfakes.FakeAuditLogRepository {
	t.Helper()
	repo := activityfakes.NewFakeAuditLogRepository()
	svc := appactivity.NewCommandService(
		activityfakes.NewFakeLoginHistoryRepository(),
		activityfakes.NewFakeServerMetricRepository(),
		repo,
		sharedfakes.NewFakeEventBus(),
	)
	ctx := ctxWithClaims("usr-1", "prf-1", false)
	_, _ = AccountAuditUnary(svc)(ctx, req, &grpc.UnaryServerInfo{FullMethod: method}, handler)
	return repo
}

// A refused password change was invisible: nothing said who tried what. The
// entry names the fields and the reason, and never carries what was typed.
func TestAccountAudit_RefusedUpdateNamesFieldsAndReason(t *testing.T) {
	mail, pw, current := "typed-address", "typed-secret-1", "typed-current-1"
	req := &eveyv1.UpdateUserRequest{Id: "usr-1", Email: &mail, Password: &pw, CurrentPassword: &current}
	var tr *eveyi18n.Translator
	repo := runAccountAudit(t, userSvcPrefix+"UpdateUser", req, func(ctx context.Context, _ any) (any, error) {
		return nil, tr.TranslatedError(ctx, codes.InvalidArgument, "errors.user.email_invalid")
	})
	got := auditEntries(t, repo, "update_user")
	if len(got) != 1 {
		t.Fatalf("%d entries, want 1", len(got))
	}
	e := got[0]
	if e.UserID != "usr-1" || e.ResourceID == nil || *e.ResourceID != "usr-1" {
		t.Errorf("entry = %+v, want the caller and the target account", e)
	}
	if e.Details["outcome"] != "refused" || e.Details["code"] != "InvalidArgument" || e.Details["reason"] != "errors.user.email_invalid" {
		t.Errorf("details = %v, want refused / InvalidArgument / errors.user.email_invalid", e.Details)
	}
	if fields, _ := e.Details["fields"].([]string); !slices.Equal(fields, []string{"current_password", "email", "password"}) {
		t.Errorf("fields = %v, want the three field names", e.Details["fields"])
	}
	dump := fmt.Sprint(e.Details)
	for _, typed := range []string{mail, pw, current} {
		if strings.Contains(dump, typed) {
			t.Errorf("details %q carry a typed value %q", dump, typed)
		}
	}
}

func TestAccountAudit_CreatedProfileTakesTheResponseID(t *testing.T) {
	pin := "1234"
	req := &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "Kid", IsKids: true, Pin: &pin}
	repo := runAccountAudit(t, userSvcPrefix+"CreateProfile", req, func(context.Context, any) (any, error) {
		return &eveyv1.Profile{Id: "prf-new"}, nil
	})
	got := auditEntries(t, repo, "create_profile")
	if len(got) != 1 || got[0].ResourceID == nil || *got[0].ResourceID != "prf-new" || got[0].Details["outcome"] != "success" {
		t.Fatalf("entries = %+v, want one success on prf-new", got)
	}
	if strings.Contains(fmt.Sprint(got[0].Details), "1234") {
		t.Errorf("details %v carry the PIN", got[0].Details)
	}
}

func TestAccountAudit_UnlistedMethodWritesNothing(t *testing.T) {
	repo := runAccountAudit(t, userSvcPrefix+"ListProfiles", &eveyv1.ListProfilesRequest{}, func(context.Context, any) (any, error) {
		return &eveyv1.ListProfilesResponse{}, nil
	})
	all, err := repo.List(context.Background(), activity.AuditLogFilter{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if len(all) != 0 {
		t.Fatalf("%d entries for a read, want 0", len(all))
	}
}

// A new UserService RPC must say whether it is audited: either it changes an
// account, a profile or a way in (listed in accountAudits), or it is a read or
// a session step that audits itself.
func TestAccountAudit_EveryUserServiceMethodIsDecided(t *testing.T) {
	notAudited := map[string]bool{
		"Login": true, "Logout": true, "RefreshToken": true, // audited in the handler, or a session step
		"GetCurrentUser": true, "ListUsers": true, "ListProfiles": true, "ListAllProfiles": true,
		"ListInviteLinks": true, "ValidateInvite": true, "ListAPIKeys": true,
		"InitiateQuickConnect": true, "CheckQuickConnect": true,
	}
	for _, m := range eveyv1.UserService_ServiceDesc.Methods {
		_, audited := accountAudits[userSvcPrefix+m.MethodName]
		if audited == notAudited[m.MethodName] {
			t.Errorf("UserService.%s: add it to accountAudits or to this read list", m.MethodName)
		}
	}
}
