package grpcapi

import (
	"context"
	"os"
	"strings"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/playback"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type stubUserData struct {
	calls   int
	profile shared.ProfileID
	asked   []string
	played  map[string]bool
}

func (s *stubUserData) UserData(_ context.Context, profileID shared.ProfileID, ids []string) (map[string]playback.UserData, error) {
	s.calls++
	s.profile = profileID
	s.asked = ids
	out := map[string]playback.UserData{}
	for _, id := range ids {
		out[id] = playback.UserData{Played: s.played[id]}
	}
	return out, nil
}

func runUserData(ctx context.Context, src mediaUserDataReader, resp any) (any, error) {
	return MediaUserDataUnary(src)(ctx, nil, &grpc.UnaryServerInfo{FullMethod: "/t/T"}, func(context.Context, any) (any, error) {
		return resp, nil
	})
}

func TestMediaUserDataUnary_StampsEveryItemInOneRead(t *testing.T) {
	src := &stubUserData{played: map[string]bool{"m1": true, "s1": true}}
	resp := &eveyv1.ListMediaItemsResponse{Items: []*eveyv1.MediaItem{
		{Id: "m1", DurationMs: 1000}, {Id: "m2"}, {Id: "s1"}, {Id: "m1"},
	}}
	_, err := runUserData(pbUserCtx("u1", "profile-1"), src, resp)
	require.NoError(t, err)
	assert.Equal(t, 1, src.calls, "one read per answer, not one per item")
	assert.Equal(t, shared.ProfileID("profile-1"), src.profile, "the caller's profile")
	assert.ElementsMatch(t, []string{"m1", "m2", "s1"}, src.asked)
	for _, it := range resp.Items {
		require.NotNil(t, it.UserData, it.Id)
		assert.Equal(t, src.played[it.Id], it.UserData.Played, it.Id)
	}
	assert.Equal(t, int64(1000), resp.Items[0].UserData.DurationMs)
}

func TestMediaUserDataUnary_FindsNestedItems(t *testing.T) {
	src := &stubUserData{played: map[string]bool{"ep": true}}
	resp := &eveyv1.GetSeriesPlayTargetResponse{}
	md := resp.ProtoReflect().Descriptor()
	// Whatever field of the answer holds the MediaItem, the walk reaches it.
	var field protoreflect.FieldDescriptor
	for i := 0; i < md.Fields().Len(); i++ {
		if f := md.Fields().Get(i); f.Message() != nil && f.Message().FullName() == mediaItemFullName && !f.IsList() {
			field = f
		}
	}
	require.NotNil(t, field, "GetSeriesPlayTargetResponse carries a MediaItem")
	item := &eveyv1.MediaItem{Id: "ep"}
	resp.ProtoReflect().Set(field, protoreflect.ValueOfMessage(item.ProtoReflect()))
	_, err := runUserData(pbUserCtx("u1", "profile-1"), src, resp)
	require.NoError(t, err)
	require.NotNil(t, item.UserData)
	assert.True(t, item.UserData.Played)
}

func TestMediaUserDataUnary_LeavesOtherAnswersAlone(t *testing.T) {
	src := &stubUserData{}
	_, err := runUserData(pbUserCtx("u1", "profile-1"), src, &eveyv1.MarkAsWatchedResponse{})
	require.NoError(t, err)
	_, err = runUserData(context.Background(), src, &eveyv1.ListMediaItemsResponse{Items: []*eveyv1.MediaItem{{Id: "m1"}}})
	require.NoError(t, err)
	assert.Zero(t, src.calls, "no MediaItem, or no profile to answer for: no read")
}

// Every RPC whose answer can carry a MediaItem is one the interceptor walks:
// the list is computed from the descriptors, so a new RPC is covered the day
// it is declared. The list is pinned so a reviewer sees it grow.
func TestMediaUserDataUnary_CoversEveryRPCReturningTitles(t *testing.T) {
	var got []string
	protoregistry.GlobalFiles.RangeFilesByPackage("evey.v1", func(fd protoreflect.FileDescriptor) bool {
		for i := 0; i < fd.Services().Len(); i++ {
			ms := fd.Services().Get(i).Methods()
			for j := 0; j < ms.Len(); j++ {
				if mayHoldMediaItem(ms.Get(j).Output()) {
					got = append(got, string(ms.Get(j).FullName()))
				}
			}
		}
		return true
	})
	assert.ElementsMatch(t, []string{
		"evey.v1.LibraryService.GetMediaItem",
		"evey.v1.LibraryService.ListMediaItems",
		"evey.v1.LibraryService.SearchMedia",
		"evey.v1.LibraryService.FilterMediaItems",
		"evey.v1.PlaybackService.GetResumeItems",
		"evey.v1.PlaybackService.GetNextUp",
		"evey.v1.PlaybackService.GetSeriesPlayTarget",
	}, got)
}

// The interceptor only works where it is chained: the production server builds
// its chain in cmd/evey/grpc.go.
func TestMediaUserDataUnary_IsInTheServerChain(t *testing.T) {
	src, err := os.ReadFile("../../../../cmd/evey/grpc.go")
	require.NoError(t, err)
	chain := string(src)
	start := strings.Index(chain, "unary := []grpc.UnaryServerInterceptor{")
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(chain[start:], "\n\t}\n")
	require.Greater(t, end, 0)
	assert.Contains(t, chain[start:start+end], "grpcapi.MediaUserDataUnary(", "the unary chain stamps the watched mark")
}
