package httpapi

// cast_next_handler.go - the cast receiver's next-episode door.
//
// A Chromecast has no user input and no JWT: once the sender's phone goes to
// sleep, the receiver's only identity is its cast stream token. Without this
// endpoint the TV plays its episode to the end and stops - the viewer must
// wake the phone to continue (real complaint, 2026-08-07). The receiver calls
// this near the end of playback; the response names the next episode and
// carries a fresh cast token for it, so the receiver can swap media on its
// own. The current token IS the authorization: it only ever unlocks the
// single immediately-next episode, re-checked against library access and
// parental rules exactly like prewarm-next (same IDOR/pivot surface).

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	domainuser "github.com/eveyproject/evey/internal/domain/user"
	"github.com/eveyproject/evey/internal/pkg/tracing"
	"go.opentelemetry.io/otel/attribute"
)

type castNextResponse struct {
	NextMediaItemID string `json:"next_media_item_id"`
	Token           string `json:"token"`
	Title           string `json:"title"`
	SeasonNumber    int    `json:"season_number"`
	EpisodeNumber   int    `json:"episode_number"`
	SeriesTitle     string `json:"series_title"`
}

// serveCastNext handles POST /api/v1/media/{id}/cast-next (streamAuth).
// Receiver-only by design: a JWT caller has the full gRPC surface for
// episode navigation and must not use this token mint as a shortcut.
// 204 = nothing to chain (movie, series finale, next file missing).
func (h *hlsVODServe) serveCastNext(w http.ResponseWriter, r *http.Request) {
	ctx, span := tracing.StartSpan(r.Context(), "http.transcode.CastNext")
	defer span.End()
	r = r.WithContext(ctx)

	currentID := r.PathValue("id")
	st, ok := StreamTokenFromContext(r.Context())
	if !ok || st == nil || !st.Cast {
		http.Error(w, `{"message":"cast token required"}`, http.StatusForbidden)
		return
	}
	// Pinned to its own media: a leaked token must not walk arbitrary series.
	if currentID == "" || string(st.MediaItemID) != currentID {
		http.Error(w, `{"message":"token not valid for this media"}`, http.StatusForbidden)
		return
	}
	span.SetAttributes(attribute.String("media_item_id", currentID))
	// The warm-up path below reads claims (profile prefs); streamAuth already
	// derives them from the token, but this handler must not depend on that
	// registration detail - derive them here from the same token.
	r = r.WithContext(context.WithValue(r.Context(), httpClaimsCtxKey{},
		&domainuser.Claims{UserID: string(st.UserID), ProfileID: string(st.ProfileID)}))

	curItem, err := h.libSvc.GetMediaItem(r.Context(), currentID)
	if err != nil || curItem == nil {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if curItem.Type != "episode" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	next, err := h.libSvc.NextEpisodeOf(r.Context(), currentID)
	if err != nil {
		span.RecordError(err)
		slog.WarnContext(r.Context(), "cast-next: next episode lookup failed", slog.String("current_id", currentID), slog.Any("err", err))
		http.Error(w, `{"message":"lookup failed"}`, http.StatusInternalServerError)
		return
	}
	if next == nil || next.File == nil || next.File.Path == "" {
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Access re-checked on the NEXT item with the token's identity - the
	// current episode's access does not imply the next one (series straddling
	// libraries, kid profile blocked on ep N+1). 404 keeps existence hidden,
	// mirroring checkLibraryAccess. Nil checker = deny (fail closed).
	if h.libraryAccess == nil {
		slog.WarnContext(r.Context(), "cast-next: library access checker is nil, denying", slog.String("next_id", string(next.ID)))
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if allowed, aErr := h.libraryAccess.HasAccess(r.Context(), st.UserID, next.LibraryID); aErr != nil || !allowed {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}
	if h.parental != nil && h.parental.BlockedByParental(r.Context(), string(st.ProfileID), next) {
		http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		return
	}

	if h.streamTokens == nil {
		http.Error(w, `{"message":"cast-next unavailable"}`, http.StatusServiceUnavailable)
		return
	}
	token := h.streamTokens.CreateCast(st.UserID, st.ProfileID, next.ID, appmedia.CastStreamTokenTTL)

	// Overlay metadata via the parent chain (episode → season → series).
	// Best-effort: a hole in the chain degrades the label, never the switch.
	resp := castNextResponse{
		NextMediaItemID: string(next.ID),
		Token:           token,
		Title:           next.Title,
	}
	if next.IndexNumber != nil {
		resp.EpisodeNumber = *next.IndexNumber
	}
	if next.ParentID != "" {
		if season, sErr := h.libSvc.GetMediaItem(r.Context(), next.ParentID); sErr == nil && season != nil {
			if season.IndexNumber != nil {
				resp.SeasonNumber = *season.IndexNumber
			}
			if season.ParentID != "" {
				if series, srErr := h.libSvc.GetMediaItem(r.Context(), season.ParentID); srErr == nil && series != nil {
					resp.SeriesTitle = series.Title
				}
			}
		}
	}

	// Best-effort first-segment warm-up so the switch lands on a hot cache
	// (same rafale prewarm-next fires for the local players). The receiver
	// passes its master's query params, so the fingerprint matches the
	// master request it will make. Failure only costs switch latency.
	if streams, wErr := h.libSvc.GetMediaStreams(r.Context(), string(next.ID)); wErr == nil {
		if fp, fErr := h.buildFingerprint(r, string(next.ID), next, streams); fErr == nil {
			if producer := h.producerForRequest(r.Context(), r, streams, next); producer != nil {
				if eErr := producer.EnsureSegmentRange(r.Context(), fp, 0, 0); eErr != nil {
					slog.InfoContext(r.Context(), "cast-next: prewarm skipped", slog.String("next_id", string(next.ID)), slog.Any("err", eErr))
				}
			}
		}
	}

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(resp); err != nil {
		slog.WarnContext(r.Context(), "cast-next: encode failed", slog.Any("err", err))
	}
}
