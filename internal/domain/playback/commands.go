package playback

import "context"

type ProgressCommandRepo interface {
	Upsert(ctx context.Context, wp *WatchProgress) error
}

type SessionCommandRepo interface {
	Create(ctx context.Context, session *PlaySession) error
	Update(ctx context.Context, session *PlaySession) error
}
