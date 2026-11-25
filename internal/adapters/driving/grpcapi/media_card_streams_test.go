package grpcapi

import (
	"context"
	"os"
	"strings"
	"testing"

	eveyv1 "github.com/eveyproject/evey/gen/evey/v1"
	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
)

type stubStreams struct {
	calls int
	asked []string
	by    map[string][]*library.MediaStream
}

func (s *stubStreams) GetStreamsForItems(_ context.Context, ids []string) (map[string][]*library.MediaStream, error) {
	s.calls++
	s.asked = ids
	return s.by, nil
}

func uhdHDR() []*library.MediaStream {
	w, h, transfer := 3840, 2160, "smpte2084"
	return []*library.MediaStream{
		{StreamType: library.StreamTypeAudio, Codec: "eac3"},
		{StreamType: library.StreamTypeVideo, Codec: "hevc", Width: &w, Height: &h, ColorTransfer: &transfer},
	}
}

func runCardStreams(src mediaStreamsReader, resp any) error {
	_, err := MediaCardStreamsUnary(src)(context.Background(), nil, &grpc.UnaryServerInfo{FullMethod: "/t/T"}, func(context.Context, any) (any, error) {
		return resp, nil
	})
	return err
}

func TestMediaCardStreamsUnary_FillsEveryCardInOneRead(t *testing.T) {
	src := &stubStreams{by: map[string][]*library.MediaStream{"film": uhdHDR()}}
	feedResp := &eveyv1.FeedResponse{Items: []*eveyv1.FeedItem{{MediaItemId: "film"}, {MediaItemId: "series"}, {MediaItemId: "film"}}}
	require.NoError(t, runCardStreams(src, feedResp))
	assert.Equal(t, 1, src.calls)
	assert.ElementsMatch(t, []string{"film", "series"}, src.asked)
	v := feedResp.Items[0].GetVideoStream()
	require.NotNil(t, v, "the video stream, not the first stream")
	assert.Equal(t, int32(3840), v.GetWidth())
	assert.Equal(t, int32(2160), v.GetHeight())
	assert.Equal(t, "smpte2084", v.GetColorTransfer())
	assert.Nil(t, feedResp.Items[1].GetVideoStream(), "a series has no stream of its own: no badge")

	popular := &eveyv1.GetPopularMediaResponse{Items: []*eveyv1.PopularMediaItem{{MediaItemId: "film"}}}
	require.NoError(t, runCardStreams(src, popular))
	require.NotNil(t, popular.Items[0].GetVideoStream())
}

func TestMediaCardStreamsUnary_LeavesOtherAnswersAlone(t *testing.T) {
	src := &stubStreams{}
	require.NoError(t, runCardStreams(src, &eveyv1.MarkAsWatchedResponse{}))
	require.NoError(t, runCardStreams(src, &eveyv1.FeedResponse{}))
	assert.Zero(t, src.calls)
}

// The card messages are found by their shape (media_item_id + video_stream),
// so a new rail is covered the day it declares the pair. Pinned so a reviewer
// sees the list grow.
func TestMediaCardStreamsUnary_KnowsEveryCardMessage(t *testing.T) {
	var got []string
	protoregistry.GlobalTypes.RangeMessages(func(mt protoreflect.MessageType) bool {
		if md := mt.Descriptor(); md.ParentFile().Package() == "evey.v1" && isCardMessage(md) {
			got = append(got, string(md.FullName()))
		}
		return true
	})
	assert.ElementsMatch(t, []string{
		"evey.v1.FeedItem", "evey.v1.PopularMediaItem", "evey.v1.FavoriteProto", "evey.v1.CollectionItemProto",
	}, got)
}

func TestMediaCardStreamsUnary_IsInTheServerChain(t *testing.T) {
	src, err := os.ReadFile("../../../../cmd/evey/grpc.go")
	require.NoError(t, err)
	chain := string(src)
	start := strings.Index(chain, "unary := []grpc.UnaryServerInterceptor{")
	require.GreaterOrEqual(t, start, 0)
	end := strings.Index(chain[start:], "\n\t}\n")
	require.Positive(t, end)
	assert.Contains(t, chain[start:start+end], "grpcapi.MediaCardStreamsUnary(", "the unary chain fills the card badges")
}

// A favourite names its title by media_item_id (its own id is the favourite's);
// a collection's title by id.
func TestMediaCardStreamsUnary_FavouritesAndCollectionsByTheirTitle(t *testing.T) {
	src := &stubStreams{by: map[string][]*library.MediaStream{"film": uhdHDR()}}
	favs := &eveyv1.ListFavoritesResponse{Items: []*eveyv1.FavoriteProto{{Id: "fav-1", MediaItemId: "film"}}}
	require.NoError(t, runCardStreams(src, favs))
	assert.Equal(t, []string{"film"}, src.asked)
	require.NotNil(t, favs.Items[0].GetVideoStream())

	col := &eveyv1.CollectionItemProto{Id: "film"}
	require.NoError(t, runCardStreams(src, &eveyv1.GetCollectionResponse{Items: []*eveyv1.CollectionItemProto{col}}))
	require.NotNil(t, col.GetVideoStream())
}

// A title sent without streams (a search, Continue Watching) gets its video
// stream only; one that has its streams is left as it is, and not read.
func TestMediaCardStreamsUnary_BareTitlesGetTheirVideoStream(t *testing.T) {
	src := &stubStreams{by: map[string][]*library.MediaStream{"ep": uhdHDR()}}
	resp := &eveyv1.SearchMediaResponse{Items: []*eveyv1.MediaItem{{Id: "ep"}, {Id: "full", Streams: []*eveyv1.MediaStream{{Index: 7}}}}}
	require.NoError(t, runCardStreams(src, resp))
	require.Len(t, resp.Items[0].Streams, 1)
	assert.Equal(t, int32(2160), resp.Items[0].Streams[0].GetHeight())
	assert.Equal(t, []string{"ep"}, src.asked)
	assert.Equal(t, int32(7), resp.Items[1].Streams[0].GetIndex())
}
