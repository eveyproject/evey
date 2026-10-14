package fakes

import (
	"context"
	"fmt"
	"sync"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

type FakeMediaStreamRepository struct {
	mu    sync.RWMutex
	items map[string][]*library.MediaStream
	Err   error
}

func NewFakeMediaStreamRepository() *FakeMediaStreamRepository {
	return &FakeMediaStreamRepository{items: make(map[string][]*library.MediaStream)}
}

func (r *FakeMediaStreamRepository) CreateBatch(_ context.Context, streams []*library.MediaStream) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	for _, s := range streams {
		r.items[string(s.MediaItemID)] = append(r.items[string(s.MediaItemID)], s)
	}
	return nil
}

func (r *FakeMediaStreamRepository) ReplaceBatch(_ context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.items, string(mediaItemID))
	for _, s := range streams {
		r.items[string(s.MediaItemID)] = append(r.items[string(s.MediaItemID)], s)
	}
	return nil
}

func (r *FakeMediaStreamRepository) GetByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) ([]*library.MediaStream, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	return r.items[string(mediaItemID)], nil
}

func (r *FakeMediaStreamRepository) GetByMediaItemIDs(_ context.Context, mediaItemIDs []shared.MediaItemID) (map[shared.MediaItemID][]*library.MediaStream, error) {
	if r.Err != nil {
		return nil, r.Err
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.Err != nil {
		return nil, r.Err
	}
	result := make(map[shared.MediaItemID][]*library.MediaStream, len(mediaItemIDs))
	for _, id := range mediaItemIDs {
		if streams, ok := r.items[string(id)]; ok {
			result[id] = streams
		}
	}
	return result, nil
}

func (r *FakeMediaStreamRepository) DeleteByMediaItemID(_ context.Context, mediaItemID shared.MediaItemID) error {
	if r.Err != nil {
		return r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Err != nil {
		return r.Err
	}
	delete(r.items, string(mediaItemID))
	return nil
}

// RefreshProbedFields mirrors the Postgres contract: same container streams
// (index, type, codec) or ErrStreamSetChanged, then only the probe's own
// fields are written, a nil one keeping the stored value.
func (r *FakeMediaStreamRepository) RefreshProbedFields(_ context.Context, mediaItemID shared.MediaItemID, streams []*library.MediaStream) (library.RefreshResult, error) {
	var res library.RefreshResult
	if r.Err != nil {
		return res, r.Err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	stored := map[int]*library.MediaStream{}
	for _, s := range r.items[string(mediaItemID)] {
		if !s.IsExternal {
			stored[s.StreamIndex] = s
		}
	}
	var probed []*library.MediaStream
	for _, s := range streams {
		if !s.IsExternal {
			probed = append(probed, s)
		}
	}
	if len(probed) != len(stored) {
		return res, library.ErrStreamSetChanged
	}
	for _, p := range probed {
		if s, ok := stored[p.StreamIndex]; !ok || s.StreamType != p.StreamType {
			return res, library.ErrStreamSetChanged
		}
	}
	for _, p := range probed {
		s := stored[p.StreamIndex]
		before := *s
		refreshField(&s.Framerate, p.Framerate)
		refreshField(&s.BitDepth, p.BitDepth)
		refreshField(&s.PixelFormat, p.PixelFormat)
		refreshField(&s.ColorSpace, p.ColorSpace)
		refreshField(&s.ColorTransfer, p.ColorTransfer)
		refreshField(&s.HDRType, p.HDRType)
		refreshField(&s.Bitrate, p.Bitrate)
		refreshField(&s.Channels, p.Channels)
		refreshField(&s.ChannelLayout, p.ChannelLayout)
		codec, keptCodec := library.RefreshCodec(s.Codec, p.Codec)
		if keptCodec {
			res.Kept = append(res.Kept, fmt.Sprintf("stream %d codec: stored %q, probe %q", p.StreamIndex, s.Codec, p.Codec))
		}
		title, keptTitle := library.RefreshTitle(s.Title, p)
		if keptTitle {
			res.Kept = append(res.Kept, fmt.Sprintf("stream %d title: stored %q, probe %q", p.StreamIndex, s.Title, p.Title))
		}
		s.Codec, s.Title = codec, title
		if !sameProbedFields(&before, s) {
			res.Changed++
		}
	}
	return res, nil
}

func refreshField[T comparable](stored **T, probed *T) {
	if probed != nil {
		v := *probed
		*stored = &v
	}
}

func sameProbedFields(a, b *library.MediaStream) bool {
	return eqPtr(a.Framerate, b.Framerate) && eqPtr(a.BitDepth, b.BitDepth) && eqPtr(a.PixelFormat, b.PixelFormat) &&
		eqPtr(a.ColorSpace, b.ColorSpace) && eqPtr(a.ColorTransfer, b.ColorTransfer) && eqPtr(a.HDRType, b.HDRType) &&
		eqPtr(a.Bitrate, b.Bitrate) && eqPtr(a.Channels, b.Channels) && eqPtr(a.ChannelLayout, b.ChannelLayout) &&
		a.Codec == b.Codec && a.Title == b.Title
}

func eqPtr[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}
