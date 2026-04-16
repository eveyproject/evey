package grpcapi

import (
	"context"
	"testing"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// GetAnalytics requires admin claims.
func TestActivityHandler_GetAnalytics_NonAdmin_PermissionDenied(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	_, err := h.GetAnalytics(ctx, &eveyv1.GetAnalyticsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// GetAnalytics without claims returns PermissionDenied (no claims ≠ admin).
func TestActivityHandler_GetAnalytics_NoClaims_PermissionDenied(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	_, err := h.GetAnalytics(context.Background(), &eveyv1.GetAnalyticsRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.PermissionDenied, status.Code(err))
}

// GetAnalytics with admin returns the wire payload; the fake service yields
// zero-valued totals (no history) plus empty slices.
func TestActivityHandler_GetAnalytics_Admin_Empty(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("admin-1", "profile-1", true)
	resp, err := h.GetAnalytics(ctx, &eveyv1.GetAnalyticsRequest{})
	require.NoError(t, err)
	assert.Equal(t, int32(0), resp.TotalPlaysToday)
	assert.Equal(t, int32(0), resp.TotalPlaysThisWeek)
	assert.Equal(t, int32(0), resp.ActiveUsersToday)
	assert.Empty(t, resp.TopMedia)
	assert.Empty(t, resp.RecentActivity)
}

// GetPopularMedia requires auth.
func TestActivityHandler_GetPopularMedia_Unauthenticated(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	_, err := h.GetPopularMedia(context.Background(), &eveyv1.GetPopularMediaRequest{})
	require.Error(t, err)
	assert.Equal(t, codes.Unauthenticated, status.Code(err))
}

// GetPopularMedia with no data returns empty list (no lib query wired).
func TestActivityHandler_GetPopularMedia_EmptyResults(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetPopularMedia(ctx, &eveyv1.GetPopularMediaRequest{Limit: 10})
	require.NoError(t, err)
	assert.Empty(t, resp.Items)
}

// GetPopularMedia with limit <= 0 defaults to 20 internally — we can't assert
// the default directly, but we can assert the call succeeds.
func TestActivityHandler_GetPopularMedia_DefaultLimit(t *testing.T) {
	t.Parallel()
	h, _, _, _, _ := newActivityHandler()
	ctx := ctxWithClaims("user-1", "profile-1", false)
	resp, err := h.GetPopularMedia(ctx, &eveyv1.GetPopularMediaRequest{Limit: 0})
	require.NoError(t, err)
	assert.NotNil(t, resp)
}
