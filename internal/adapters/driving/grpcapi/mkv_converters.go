package grpcapi

import (
	"fmt"

	eveyv1 "github.com/evey-project/evey/gen/evey/v1"
	"github.com/evey-project/evey/internal/domain/mkv"
)

func mkvChapterToProto(ch mkv.Chapter) *eveyv1.Chapter {
	return &eveyv1.Chapter{
		Id:      fmt.Sprintf("%d", ch.ID),
		Title:   ch.Title,
		StartMs: ch.StartMs,
		EndMs:   ch.EndMs,
	}
}
