package grpcapi

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/activity"
	libfakes "github.com/eveyproject/evey/internal/domain/library/fakes"
	userfakes "github.com/eveyproject/evey/internal/domain/user/fakes"
)

// A page that comes back exactly full must hand out a cursor: `>=` read as `>`
// (it survived mutation testing on 2026-10-18) ended the history after its
// first page whenever the page size divided the count.
func TestActivityHandler_GetPlayHistory_CursorWalksEveryEntry(t *testing.T) {
	h, _, _, playHistory, _ := newActivityHandler()
	wireActivityAccess(h, libfakes.NewFakeMediaItemQueryRepo(), userfakes.NewFakeProfileRepository())

	now := time.Now()
	for i := range 4 {
		playHistory.Add(&activity.PlayHistoryEntry{
			ID: fmt.Sprintf("ph%d", i), ProfileID: "profile-1",
			MediaItemID: "m1", MediaTitle: "Movie", MediaType: "movie",
			StartedAt: now.Add(-time.Duration(i) * time.Minute),
		})
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	var seen []string
	cursor := ""
	for range 5 {
		resp, err := h.GetPlayHistory(ctx, &eveyv1.GetPlayHistoryRequest{
			Pagination: &eveyv1.PaginationRequest{PageSize: 2, Cursor: cursor},
		})
		require.NoError(t, err)
		for _, e := range resp.Entries {
			seen = append(seen, e.Id)
		}
		cursor = resp.GetPagination().GetNextCursor()
		if cursor == "" {
			break
		}
	}
	assert.Equal(t, []string{"ph0", "ph1", "ph2", "ph3"}, seen)
}

// Same rule on the login history, whose first page goes through the offset
// query and every later one through the cursor query.
func TestActivityHandler_GetLoginHistory_CursorWalksEveryEntry(t *testing.T) {
	h, loginRepo, _, _, _ := newActivityHandler()

	now := time.Now()
	for i := range 4 {
		require.NoError(t, loginRepo.Create(context.Background(), &activity.LoginEntry{
			ID: fmt.Sprintf("l%d", i), UserID: "user-1", Success: true,
			CreatedAt: now.Add(-time.Duration(i) * time.Minute),
		}))
	}

	ctx := ctxWithClaims("user-1", "profile-1", false)
	var seen []string
	cursor := ""
	for range 5 {
		resp, err := h.GetLoginHistory(ctx, &eveyv1.GetLoginHistoryRequest{
			Pagination: &eveyv1.PaginationRequest{PageSize: 2, Cursor: cursor},
		})
		require.NoError(t, err)
		for _, e := range resp.Entries {
			seen = append(seen, e.Id)
		}
		cursor = resp.GetPagination().GetNextCursor()
		if cursor == "" {
			break
		}
	}
	assert.Equal(t, []string{"l0", "l1", "l2", "l3"}, seen)
}
