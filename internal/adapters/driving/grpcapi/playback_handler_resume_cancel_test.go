package grpcapi

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
)

// Leaving the home page while the resume rail loads cancels the request. That
// is the client giving up, not a server fault: it must answer Canceled, not
// Internal, or every such navigation is an ERROR line and a 5xx in the metrics.
func TestPlaybackHandler_GetResumeItems_ClientCancelIsNotInternal(t *testing.T) {
	for name, cause := range map[string]error{
		"context canceled":    context.Canceled,
		"pq statement cancel": errors.New("pq: canceling statement due to user request (57014)"),
	} {
		t.Run(name, func(t *testing.T) {
			h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
			progressRepo.Err = fmt.Errorf("WatchProgressRepo.GetResumeItems: %w", cause)

			_, err := h.GetResumeItems(pbAdminCtx(), &eveyv1.GetResumeItemsRequest{Limit: 10})
			require.Error(t, err)
			assert.Equal(t, codes.Canceled, status.Code(err))
		})
	}

	h, _, progressRepo, _, _, _ := newTestPlaybackHandler()
	progressRepo.Err = errors.New("WatchProgressRepo.GetResumeItems: connection refused")
	_, err := h.GetResumeItems(pbAdminCtx(), &eveyv1.GetResumeItemsRequest{Limit: 10})
	assert.Equal(t, codes.Internal, status.Code(err), "a real database failure stays Internal")
}
