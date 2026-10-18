package grpcapi

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"testing"
	"time"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/activity"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/domain/user"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
	"github.com/eveyproject/evey/internal/pkg/auth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

// These tests run the handler behind the REAL AuthInterceptor, authenticated by
// an API key whose owner is an admin. The interceptor lets each call through,
// since the key holds the method's scope; what is under test is the handler's
// admin-only branch, which used to read claims.IsAdmin - true for every
// admin-owned key, whatever it was scoped for.

// viaAPIKey calls call with the context the interceptor builds for a key owned
// by ownerID (read from users) and carrying perms, on method.
func viaAPIKey(t *testing.T, users user.UserRepository, ownerID shared.UserID, perms []string, method string, call func(ctx context.Context) error) error {
	t.Helper()
	const raw = "evey_admin_reach"
	sum := sha256.Sum256([]byte(raw))
	keys := userfakes.NewFakeAPIKeyRepository()
	require.NoError(t, keys.Create(context.Background(), &user.APIKeyModel{
		ID: "ak-reach", UserID: ownerID, Name: "automation",
		KeyHash: hex.EncodeToString(sum[:]), Permissions: perms,
	}))
	ic := interceptors.NewAuthInterceptor(userfakes.NewFakeTokenGenerator()).WithAPIKeys(keys, users).Unary()
	ctx := metadata.NewIncomingContext(context.Background(), metadata.Pairs("authorization", raw))
	_, err := ic(ctx, nil, &grpc.UnaryServerInfo{FullMethod: method}, func(ctx context.Context, _ any) (any, error) {
		require.True(t, interceptors.ClaimsFromContext(ctx).IsAdmin, "the key's claims carry its owner's admin flag")
		return nil, call(ctx)
	})
	return err
}

func seedAdminKeyOwner(t *testing.T, env *testUserEnv) {
	t.Helper()
	seedUser(t, env, &user.User{ID: "usr-root", Username: "root", Email: "r@b.com", IsAdmin: true}, "pass12345678")
}

// A users:write key is the key owner's account and profiles. Resetting someone
// else's password is account lifecycle, which no API key reaches (CreateUser,
// DeleteUser, DisableUser are InternalMethods; ImpersonateUser refuses keys).
func TestAPIKey_AdminUsersWriteKeyCannotResetAnotherPassword(t *testing.T) {
	env := setupUserHandler()
	seedAdminKeyOwner(t, env)
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "oldpassword")

	newPass := "attackerpass1"
	promote := true
	err := viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersWrite}, "/evey.v1.UserService/UpdateUser",
		func(ctx context.Context) error {
			_, callErr := env.handler.UpdateUser(ctx, &eveyv1.UpdateUserRequest{Id: "usr-1", Password: &newPass, IsAdmin: &promote})
			return callErr
		})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))

	u, gErr := env.userRepo.GetByID(context.Background(), "usr-1")
	require.NoError(t, gErr)
	assert.Equal(t, "hashed:oldpassword", u.PasswordHash, "the other account's password must be untouched")
	assert.False(t, u.IsAdmin)
}

func TestAPIKey_AdminUsersWriteKeyCannotManageAnotherAccountsProfiles(t *testing.T) {
	env := setupUserHandler()
	seedAdminKeyOwner(t, env)
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-kid", UserID: "usr-1", Name: "Kid", IsKids: true}))
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-spare", UserID: "usr-1", Name: "Spare"}))

	err := viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersWrite}, "/evey.v1.UserService/CreateProfile",
		func(ctx context.Context) error {
			_, callErr := env.handler.CreateProfile(ctx, &eveyv1.CreateProfileRequest{UserId: "usr-1", Name: "Planted"})
			return callErr
		})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "CreateProfile for another account")

	notKids := false
	err = viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersWrite}, "/evey.v1.UserService/UpdateProfile",
		func(ctx context.Context) error {
			_, callErr := env.handler.UpdateProfile(ctx, &eveyv1.UpdateProfileRequest{Id: "prf-kid", IsKids: &notKids})
			return callErr
		})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "UpdateProfile on another account's kids profile")
	kid, _ := env.profileRepo.GetByID(context.Background(), "prf-kid")
	require.NotNil(t, kid)
	assert.True(t, kid.IsKids, "the kids profile keeps its parental control")

	err = viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersWrite}, "/evey.v1.UserService/DeleteProfile",
		func(ctx context.Context) error {
			_, callErr := env.handler.DeleteProfile(ctx, &eveyv1.DeleteProfileRequest{Id: "prf-spare"})
			return callErr
		})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "DeleteProfile on another account")
	spare, _ := env.profileRepo.GetByID(context.Background(), "prf-spare")
	assert.NotNil(t, spare, "the other account's profile survives")
}

// Creating an invite is RoleAdmin with no API-key scope, so no key reaches it;
// revoking any admin's invite through the users:write bucket was the back door.
func TestAPIKey_AdminUsersWriteKeyCannotRevokeAnotherAdminsInvite(t *testing.T) {
	env := setupUserHandler()
	seedAdminKeyOwner(t, env)
	link, err := env.handler.CreateInviteLink(adminCtx(), &eveyv1.CreateInviteLinkRequest{MaxUses: 1})
	require.NoError(t, err)

	err = viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersWrite}, "/evey.v1.UserService/RevokeInviteLink",
		func(ctx context.Context) error {
			_, callErr := env.handler.RevokeInviteLink(ctx, &eveyv1.RevokeInviteLinkRequest{Id: link.Id})
			return callErr
		})
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// Stopping or driving another viewer's session is what AdminSessionService
// does (KillSession, SendSessionCommand), and those have no API-key scope.
func TestAPIKey_AdminPlaybackWriteKeyCannotTouchAnotherSession(t *testing.T) {
	h, _, _, _, _, _ := newTestPlaybackHandler()
	started, err := h.StartStream(pbAdminCtx(), &eveyv1.StartStreamRequest{
		MediaItemId: "mi-1", PlayMethod: eveyv1.PlayMethod_PLAY_METHOD_DIRECT_PLAY, DeviceName: "TV",
	})
	require.NoError(t, err)

	users := userfakes.NewFakeUserRepository()
	require.NoError(t, users.Create(context.Background(), &user.User{ID: "usr-root", Username: "root", IsAdmin: true, Enabled: true}))

	err = viaAPIKey(t, users, "usr-root", []string{auth.ScopePlaybackWrite}, "/evey.v1.PlaybackService/ReportProgress",
		func(ctx context.Context) error {
			_, callErr := h.ReportProgress(ctx, &eveyv1.ReportProgressRequest{SessionId: started.Id, PositionMs: 1})
			return callErr
		})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "ReportProgress on another viewer's session")

	err = viaAPIKey(t, users, "usr-root", []string{auth.ScopePlaybackWrite}, "/evey.v1.PlaybackService/StopStream",
		func(ctx context.Context) error {
			_, callErr := h.StopStream(ctx, &eveyv1.StopStreamRequest{SessionId: started.Id})
			return callErr
		})
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "StopStream on another viewer's session")

	// The viewer's own session keeps working.
	_, err = h.StopStream(pbAdminCtx(), &eveyv1.StopStreamRequest{SessionId: started.Id})
	require.NoError(t, err)
}

// ScanPhotoDirectory is admin-only in its handler and media:write for keys,
// like ScanLibrary. ScanLibrary is RoleAdmin, so a pre-scope key (no
// permissions) is refused there; here it passed the interceptor's legacy
// allow-all and then the bare IsAdmin test.
func TestAPIKey_UnscopedAdminKeyCannotScanPhotoDirectory(t *testing.T) {
	h, _, _, _ := newTestPhotoHandler()
	users := userfakes.NewFakeUserRepository()
	require.NoError(t, users.Create(context.Background(), &user.User{ID: "usr-root", Username: "root", IsAdmin: true, Enabled: true}))
	scan := func(ctx context.Context) error {
		_, err := h.ScanPhotoDirectory(ctx, &eveyv1.ScanPhotoDirectoryRequest{LibraryId: "lib-missing", Directory: "/media/photos"})
		return err
	}

	err := viaAPIKey(t, users, "usr-root", nil, "/evey.v1.PhotoService/ScanPhotoDirectory", scan)
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "an unscoped key must stop at the admin gate")

	// A key scoped media:write passes the gate and reaches the library lookup.
	err = viaAPIKey(t, users, "usr-root", []string{auth.ScopeMediaWrite}, "/evey.v1.PhotoService/ScanPhotoDirectory", scan)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// The symmetric case: an admin's key that DOES hold admin:write keeps the admin
// branch for another account's profiles. Only the account lifecycle (password,
// role, invites) stays JWT-only.
func TestAPIKey_AdminWriteKeyStillManagesAnotherAccountsProfiles(t *testing.T) {
	env := setupUserHandler()
	seedAdminKeyOwner(t, env)
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-spare", UserID: "usr-1", Name: "Spare"}))

	err := viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersWrite, auth.ScopeAdminWrite}, "/evey.v1.UserService/DeleteProfile",
		func(ctx context.Context) error {
			_, callErr := env.handler.DeleteProfile(ctx, &eveyv1.DeleteProfileRequest{Id: "prf-spare"})
			return callErr
		})
	require.NoError(t, err, "an admin:write key reaches the admin branch")
	spare, _ := env.profileRepo.GetByID(context.Background(), "prf-spare")
	assert.Nil(t, spare, "the profile was deleted")
}

// The read side of the same rule: an admin's users:read key reads its owner's
// login history, not another account's; with admin:read it reads both, as a
// JWT admin does.
func TestAPIKey_AdminUsersReadKeyNeedsAdminReadForAnotherLoginHistory(t *testing.T) {
	h, loginRepo, _, _, _ := newActivityHandler()
	require.NoError(t, loginRepo.Create(context.Background(), &activity.LoginEntry{
		ID: "l1", UserID: "user-2", ClientIP: "10.0.0.1", Success: true, CreatedAt: time.Now(),
	}))
	users := userfakes.NewFakeUserRepository()
	require.NoError(t, users.Create(context.Background(), &user.User{ID: "usr-root", Username: "root", IsAdmin: true, Enabled: true}))
	call := func(ctx context.Context) error {
		_, callErr := h.GetLoginHistory(ctx, &eveyv1.GetLoginHistoryRequest{UserId: "user-2"})
		return callErr
	}

	err := viaAPIKey(t, users, "usr-root", []string{auth.ScopeUsersRead}, "/evey.v1.ActivityService/GetLoginHistory", call)
	assert.Equal(t, codes.PermissionDenied, status.Code(err), "users:read alone is not another account's logins")

	err = viaAPIKey(t, users, "usr-root", []string{auth.ScopeUsersRead, auth.ScopeAdminRead}, "/evey.v1.ActivityService/GetLoginHistory", call)
	require.NoError(t, err, "admin:read reaches another account's logins")
}

func TestAPIKey_AdminUsersReadKeyListsOnlyItsOwnersProfiles(t *testing.T) {
	env := setupUserHandler()
	seedAdminKeyOwner(t, env)
	seedUser(t, env, &user.User{ID: "usr-1", Username: "alice", Email: "a@b.com"}, "pass12345678")
	require.NoError(t, env.profileRepo.Create(context.Background(), &user.Profile{ID: "prf-alice", UserID: "usr-1", Name: "Alice"}))

	var got []string
	call := func(ctx context.Context) error {
		resp, callErr := env.handler.ListProfiles(ctx, &eveyv1.ListProfilesRequest{UserId: "usr-1"})
		got = nil
		for _, p := range resp.GetProfiles() {
			got = append(got, p.Id)
		}
		return callErr
	}
	require.NoError(t, viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersRead}, "/evey.v1.UserService/ListProfiles", call))
	assert.NotContains(t, got, "prf-alice", "users:read lists its owner's profiles, not another account's")

	require.NoError(t, viaAPIKey(t, env.userRepo, "usr-root", []string{auth.ScopeUsersRead, auth.ScopeAdminRead}, "/evey.v1.UserService/ListProfiles", call))
	assert.Contains(t, got, "prf-alice", "admin:read lists another account's profiles")
}
