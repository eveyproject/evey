package ebml

import (
	"context"
	"errors"

	"github.com/gravity-zero/evey/internal/domain/mkv"
)

type Reader struct{}

func NewReader() *Reader {
	return &Reader{}
}

func (r *Reader) Open(_ context.Context, _ string) (*mkv.Container, error) {
	// TODO: parse EBML header and segment info
	return nil, errors.New("TODO: implement EBML parsing")
}

func (r *Reader) ReadTracks(_ context.Context, _ string) ([]mkv.Track, error) {
	return nil, errors.New("TODO: implement EBML track reading")
}

func (r *Reader) ReadChapters(_ context.Context, _ string) ([]mkv.Chapter, error) {
	return nil, errors.New("TODO: implement EBML chapter reading")
}

func (r *Reader) ReadAttachments(_ context.Context, _ string) ([]mkv.Attachment, error) {
	return nil, errors.New("TODO: implement EBML attachment reading")
}

func (r *Reader) ReadTags(_ context.Context, _ string) ([]mkv.Tag, error) {
	return nil, errors.New("TODO: implement EBML tag reading")
}
