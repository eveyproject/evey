package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/adapters/driving/grpcapi/interceptors"
	"github.com/eveyproject/evey/internal/domain/user"
	eveyi18n "github.com/eveyproject/evey/internal/pkg/i18n"
)

// A non-English admin reads the refusal in their own language: the messages
// come from the catalogue, never from a hard-coded English sentence.
func TestUserHandler_RefusalsAreTranslated(t *testing.T) {
	tr, err := eveyi18n.NewFromDir("../../../../i18n")
	require.NoError(t, err)
	env := setupUserHandler()
	env.handler.translator = tr
	seedUser(t, env, &user.User{ID: "adm-1", Username: "admin", IsAdmin: true}, "pass12345678")
	seedUser(t, env, &user.User{ID: "usr-off", Username: "off"}, "pass12345678")
	require.NoError(t, env.userSvc.DisableUser(context.Background(), "usr-off"))

	fr := eveyi18n.ContextWithLocale(interceptors.ContextWithClaims(context.Background(), &user.Claims{
		UserID: "adm-1", Username: "admin", IsAdmin: true, ProfileID: "prf-1",
	}), "fr")

	msg := func(err error) string { return status.Convert(err).Message() }

	_, err = env.handler.DeleteUser(fr, &eveyv1.DeleteUserRequest{Id: "ghost"})
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.user.not_found"), msg(err))
	assert.NotEqual(t, "user not found", msg(err))

	_, err = env.handler.DeleteUser(fr, &eveyv1.DeleteUserRequest{Id: "adm-1"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.user.last_admin_remove"), msg(err))

	_, err = env.handler.DisableUser(fr, &eveyv1.DisableUserRequest{Id: "adm-1", Disabled: true})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.user.last_admin_disable"), msg(err))

	_, err = env.handler.DisableUser(fr, &eveyv1.DisableUserRequest{Id: "ghost", Disabled: false})
	assert.Equal(t, tr.T("fr", "errors.user.not_found"), msg(err))

	_, err = env.handler.ImpersonateUser(fr, &eveyv1.ImpersonateUserRequest{UserId: "usr-off"})
	assert.Equal(t, codes.FailedPrecondition, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.user.disabled"), msg(err))

	_, err = env.handler.ImpersonateUser(fr, &eveyv1.ImpersonateUserRequest{UserId: "ghost"})
	assert.Equal(t, tr.T("fr", "errors.user.not_found"), msg(err))

	neg := int32(-1)
	_, err = env.handler.UpdateUser(fr, &eveyv1.UpdateUserRequest{Id: "adm-1", MaxStreams: &neg})
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Equal(t, tr.T("fr", "errors.user.limit_negative", map[string]string{"field": "max_streams"}), msg(err))
	assert.Contains(t, msg(err), "max_streams", "the offending field is named")

	// The catalogue really carries French for these, else the assertions above
	// would pass on the English fallback.
	for _, key := range []string{"errors.user.not_found", "errors.user.last_admin_remove", "errors.user.last_admin_disable", "errors.user.disabled", "errors.user.limit_negative", "errors.user.impersonation_api_key"} {
		assert.NotEqual(t, tr.T("en", key), tr.T("fr", key), key)
	}
}
