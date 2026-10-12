package main

import (
	"context"

	"github.com/eveyproject/evey/internal/domain/library"
	"github.com/eveyproject/evey/internal/domain/shared"
)

// mediaItemReaderFunc adapts a plain function to
// apptranscoding.MediaItemReader so the wire file doesn't need to declare
// a named struct for what is really just a tiny inline closure.
type mediaItemReaderFunc func(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error)

func (f mediaItemReaderFunc) GetByID(ctx context.Context, id shared.MediaItemID) (*library.MediaItem, error) {
	return f(ctx, id)
}
