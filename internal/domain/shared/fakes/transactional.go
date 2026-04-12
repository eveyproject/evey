package fakes

import "context"

// FakeTransactionalUnit runs fn inline. Supports simulating commit failure
// and observing rollback via BeforeCommit hook.
type FakeTransactionalUnit struct {
	CommitErr    error
	BeforeCommit func(ctx context.Context) error
	Calls        int
}

func NewFakeTransactionalUnit() *FakeTransactionalUnit {
	return &FakeTransactionalUnit{}
}

func (f *FakeTransactionalUnit) Do(ctx context.Context, fn func(ctx context.Context) error) error {
	f.Calls++
	if err := fn(ctx); err != nil {
		return err
	}
	if f.BeforeCommit != nil {
		if err := f.BeforeCommit(ctx); err != nil {
			return err
		}
	}
	return f.CommitErr
}
