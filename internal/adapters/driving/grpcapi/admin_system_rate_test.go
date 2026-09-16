package grpcapi

import (
	"context"
	"errors"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	appsyncplay "github.com/evey-project/evey/internal/app/syncplay"
)

// The dashboard's streaming rate is a difference between two calls: the first
// call has nothing to diff against (-1, shown as a dash), a counter that went
// backwards (restart) is -1 too, and a normal step is bytes*8/elapsed.
func TestStreamingRate(t *testing.T) {
	d := &adminSystemHandler{}
	if got := d.streamingRate(1_000); got != -1 {
		t.Errorf("first call = %d, want -1", got)
	}
	// Rewind the clock so the elapsed time is exactly known.
	d.meterLastAt = time.Now().Add(-2 * time.Second)
	d.meterLastTotal = 1_000
	got := d.streamingRate(3_000) // 2000 bytes over ~2 s = ~8000 bit/s
	if got < 7_000 || got > 9_000 {
		t.Errorf("rate = %d bit/s, want ~8000", got)
	}
	d.meterLastAt = time.Now().Add(-time.Second)
	d.meterLastTotal = 5_000
	if got := d.streamingRate(4_000); got != -1 {
		t.Errorf("counter went backwards: got %d, want -1", got)
	}
}

// Room creation: the per-user cap is the one client-actionable refusal;
// anything else is a sanitized internal error.
func TestCreateRoomError(t *testing.T) {
	ctx := context.Background()
	if got := status.Code(createRoomError(ctx, nil, appsyncplay.ErrTooManyRooms)); got != codes.ResourceExhausted {
		t.Errorf("too many rooms -> %v", got)
	}
	err := createRoomError(ctx, nil, errors.New("hub exploded"))
	if status.Code(err) != codes.Internal {
		t.Errorf("other -> %v", status.Code(err))
	}
	if st, _ := status.FromError(err); st.Message() != "internal error" {
		t.Errorf("internal detail leaked: %q", st.Message())
	}
}
