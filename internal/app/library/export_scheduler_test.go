package library

import (
	"context"
	"time"
)

func (s *Scheduler) Tick(ctx context.Context) {
	s.tick(ctx)
}

func (s *Scheduler) SetNowFunc(f func() time.Time) {
	s.nowFunc = f
}
