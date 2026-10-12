package httpapi

// picture_planner.go - the player's picture decision handed to the routes
// that draw a picture outside a session (the sync-preview clip): the same
// PlanResolver the HLS handler assembles, asked for one small SDR picture.

import (
	"context"

	apptranscoding "github.com/eveyproject/evey/internal/app/transcoding"
	domaintranscoding "github.com/eveyproject/evey/internal/domain/transcoding"
)

// pictureHeight is the tallest picture these consumers draw (an 854px clip):
// the rung a GPU backend must reach to be picked for it.
const pictureHeight = 480

// picturePlan resolves the picture route for a media item through the live
// resolver (toggles and calibration read fresh, like every plan). Unreadable
// streams yield the zero plan: software, uncorrected, never an error - the
// clip is still shown.
func (h *hlsVODServe) picturePlan(ctx context.Context, mediaID string) domaintranscoding.PicturePlan {
	if h == nil || h.libSvc == nil {
		return domaintranscoding.PicturePlan{}
	}
	streams, err := h.libSvc.GetMediaStreams(ctx, mediaID)
	if err != nil {
		return domaintranscoding.PicturePlan{}
	}
	return h.planResolver().ResolvePicture(mediaID, streams, pictureHeight)
}

// softwarePicturePlanner is the planner of a host without the HLS pipeline:
// the source corrections (tone-map, SAR, interlace) with no accelerator.
func softwarePicturePlanner(lib MediaStreamsGetter) PicturePlanner {
	if lib == nil {
		return nil
	}
	return func(ctx context.Context, mediaID string) domaintranscoding.PicturePlan {
		streams, err := lib.GetMediaStreams(ctx, mediaID)
		if err != nil {
			return domaintranscoding.PicturePlan{}
		}
		return apptranscoding.PictureFor(streams)
	}
}
