package grpcapi

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	appuser "github.com/evey-project/evey/internal/app/user"
	"github.com/evey-project/evey/internal/domain/user"
	userfakes "github.com/evey-project/evey/internal/domain/user/fakes"
)

// refreshProbe is a server stream that only carries a context: the refusal
// happens before anything is sent.
type refreshProbe struct {
	eveyv1.MetadataService_RefreshMetadataServer
	ctx context.Context
}

func (p refreshProbe) Context() context.Context    { return p.ctx }
func (p refreshProbe) SetHeader(metadata.MD) error { return nil }

// Titles, overrides and artwork are shared by every account: a profile under
// parental control (a kids profile) must not rewrite what everyone sees.
func TestMetadataWrites_RefusedFromAParentalProfile(t *testing.T) {
	ctx := context.Background()
	maxAge := 10
	profiles := userfakes.NewFakeProfileRepository()
	require.NoError(t, profiles.Create(ctx, &user.Profile{ID: "prf-kid", UserID: "usr-1", IsKids: true,
		Parental: &user.ParentalSettings{Enabled: true, MaxAge: &maxAge}}))
	require.NoError(t, profiles.Create(ctx, &user.Profile{ID: "prf-adult", UserID: "usr-1"}))
	uq := appuser.NewQueryService(userfakes.NewFakeUserRepository(), profiles,
		userfakes.NewFakeAPIKeyRepository(), userfakes.NewFakeInviteLinkRepository(),
		userfakes.NewFakeQuickConnectRepository())

	h, _, _, _ := newTestMetadataHandler()
	h.userQuery = uq

	writes := map[string]func(context.Context) error{
		"refresh": func(c context.Context) error {
			return h.RefreshMetadata(&eveyv1.RefreshMetadataRequest{MediaItemId: "m1"}, refreshProbe{ctx: c})
		},
		"set override": func(c context.Context) error {
			_, err := h.SetMetadataOverride(c, &eveyv1.SetMetadataOverrideRequest{MediaItemId: "m1", FieldName: "title", CustomValue: "x"})
			return err
		},
		"remove override": func(c context.Context) error {
			_, err := h.RemoveMetadataOverride(c, &eveyv1.RemoveMetadataOverrideRequest{Id: "mo-1"})
			return err
		},
		"re-search": func(c context.Context) error {
			_, err := h.ResearchMetadata(c, &eveyv1.ResearchMetadataRequest{MediaItemId: "m1"})
			return err
		},
	}
	restricted := func(err error) bool {
		s, _ := status.FromError(err)
		return s.Code() == codes.PermissionDenied && s.Message() == "errors.media.restricted_profile_edit"
	}
	for name, call := range writes {
		if err := call(ctxWithClaims("usr-1", "prf-kid", false)); !restricted(err) {
			t.Errorf("%s from a kids profile: got %v, want the restricted-profile refusal", name, err)
		}
		if err := call(ctxWithClaims("usr-1", "prf-adult", false)); restricted(err) {
			t.Errorf("%s from an adult profile was refused as restricted", name)
		}
		if err := call(ctxWithClaims("usr-adm", "prf-kid", true)); restricted(err) {
			t.Errorf("%s from an admin was refused as restricted", name)
		}
	}
}
