package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"

	"go.opentelemetry.io/otel/codes"

	"github.com/evey-project/evey/internal/domain/shared"
	"github.com/evey-project/evey/internal/pkg/tracing"
)

// previewEnabled reports whether "preview" (next-episode preview reel)
// segments should be served. Read live per request so the admin toggle takes
// effect without a restart; nil is treated as disabled.
//
// introAutoSkip is the all-players auto-skip toggle. For CAST callers it
// gates the intro segments themselves: the receiver auto-jumps every intro
// it receives (a TV has no input to offer a button), so off = the TV sees no
// intro segments at all. Web/mobile players (JWT or plain stream tokens)
// keep their skip button regardless and read the flag in the response's
// introAutoSkip field to auto-skip client-side. Nil = off (fail closed),
// read live per request like previewEnabled.
func segmentsHandler(svc SegmentsGetter, baseURL string, libraryAccess LibraryAccessChecker, mediaGetter MediaItemGetter, previewEnabled func() bool, parental ParentalChecker, introAutoSkip func() bool) http.HandlerFunc {
	type segmentJSON struct {
		Type       string  `json:"type"`
		StartMs    int64   `json:"startMs"`
		EndMs      int64   `json:"endMs"`
		Confidence float64 `json:"confidence"`
		Source     string  `json:"source"`
	}
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.segments.Get")
		defer span.End()
		r = r.WithContext(ctx)
		mediaID := r.PathValue("id")
		claims := HTTPClaimsFromContext(r.Context())

		castCaller := false
		if st, ok := StreamTokenFromContext(r.Context()); ok {
			if st.MediaItemID != shared.MediaItemID(mediaID) {
				span.SetStatus(codes.Error, "stream token media mismatch")
				http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
				return
			}
			castCaller = st.Cast
		}
		item, err := mediaGetter.GetMediaItem(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "media item not found")
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if !checkLibraryAccess(r.Context(), w, claims, item.LibraryID, libraryAccess) {
			span.SetStatus(codes.Error, "library access denied")
			return
		}
		if !checkParental(r.Context(), w, claims, item, parental) {
			span.SetStatus(codes.Error, "parental restricted")
			return
		}
		segments, err := svc.GetSegments(r.Context(), mediaID)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, "get segments failed")
			http.Error(w, `{"message":"internal"}`, http.StatusInternalServerError)
			return
		}

		// Optional ?types=intro,outro filter
		var typeFilter map[string]struct{}
		if raw := r.URL.Query().Get("types"); raw != "" {
			typeFilter = make(map[string]struct{})
			for _, t := range strings.Split(raw, ",") {
				if t = strings.TrimSpace(t); t != "" {
					typeFilter[t] = struct{}{}
				}
			}
		}

		// "preview" is admin-gated: even when the client asks for it, it is
		// withheld unless the server toggle is on. Default off.
		showPreview := previewEnabled != nil && previewEnabled()
		autoSkip := introAutoSkip != nil && introAutoSkip()
		withholdIntro := castCaller && !autoSkip

		out := make([]segmentJSON, 0, len(segments))
		for _, seg := range segments {
			if seg.Type == "preview" && !showPreview {
				continue
			}
			if seg.Type == "intro" && withholdIntro {
				continue
			}
			if typeFilter != nil {
				if _, ok := typeFilter[seg.Type]; !ok {
					continue
				}
			}
			out = append(out, segmentJSON{
				Type:       seg.Type,
				StartMs:    seg.StartMs,
				EndMs:      seg.EndMs,
				Confidence: seg.Confidence,
				Source:     seg.Source,
			})
		}
		w.Header().Set("Content-Type", "application/json")
		setCORSHeaders(w, r, baseURL)
		// introAutoSkip lets web/mobile players auto-skip client-side; the
		// cast receiver ignores it (its intros are gated above instead).
		if err := json.NewEncoder(w).Encode(map[string]any{"segments": out, "introAutoSkip": autoSkip}); err != nil {
			slog.WarnContext(r.Context(), "json encode failed", slog.Any("err", err))
		}
	}
}
