package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/eveyproject/evey/internal/pkg/metrics"
	"github.com/eveyproject/evey/internal/pkg/ratelimit"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/shared"
	"github.com/eveyproject/evey/internal/pkg/tracing"
)

// The account JWT used to ride in ZIP-download URLs (collections, playlists,
// music tracks/albums): an <a href> cannot send an Authorization header, so
// the frontend appended ?token=<JWT> - and reverse-proxy access logs plus
// browser history then held a replayable session credential. The media
// download path solved this long ago with media-scoped stream tokens; this
// file extends the same trade to every other download resource: POST here
// once (JWT in a header), get a short-lived opaque token bound to ONE
// resource id, put THAT in the URL.
//
// The mint deliberately performs no per-resource ACL: the token only carries
// the minting user's identity (the download handler re-runs its own access
// checks on the synthesized claims), and binds to a single id (the guard
// below refuses it anywhere else). Minting a token for a resource you cannot
// access yields a token that downloads nothing.
func downloadTokenHandler(store *appmedia.StreamTokenStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ctx, span := tracing.StartSpan(r.Context(), "http.download.MintToken")
		defer span.End()
		r = r.WithContext(ctx)
		claims := HTTPClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, `{"message":"unauthorized"}`, http.StatusUnauthorized)
			return
		}
		id := r.PathValue("id")
		// Same bounds the gRPC request-id filter applies: this value lands in
		// URLs and logs, an unbounded one is an injection/abuse vector.
		if id == "" || len(id) > 128 {
			http.Error(w, `{"message":"bad request"}`, http.StatusBadRequest)
			return
		}
		token := store.Create(shared.UserID(claims.UserID), shared.ProfileID(claims.ProfileID), shared.MediaItemID(id))
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]string{"token": token}); err != nil {
			slog.WarnContext(r.Context(), "download token encode failed", slog.Any("err", err))
		}
	}
}

// requireDownloadTokenBinding is the download handlers' guard when the
// request authenticated via an opaque store token: the token must be bound to
// THIS resource id (a media-scoped stream token must not download an
// unrelated collection), and a syncplay guest may stream but never download.
// JWT-authenticated requests (no store token in ctx) pass untouched. Returns
// false after writing the refusal.
func requireDownloadTokenBinding(w http.ResponseWriter, r *http.Request, resourceID string) bool {
	st, ok := StreamTokenFromContext(r.Context())
	if !ok || st == nil {
		return true
	}
	if st.Scope == shared.ScopeSyncplayGuest || st.MediaItemID != shared.MediaItemID(resourceID) {
		// Counted and logged, like every other authorization refusal in this
		// package: a token replayed against a resource it was not minted for
		// is either a client bug or somebody probing with a leaked token, and
		// a silent 403 leaves the operator nothing to find when the user says
		// "my download does not work". The bound id is NOT logged - it is the
		// caller's input, and the useful fact is which resource refused.
		metrics.AuthzDeniedTotal.WithLabelValues("http", "download_token_scope_mismatch").Inc()
		slog.WarnContext(r.Context(), "download refused: the stream token is not bound to this resource",
			slog.String("resource_id", resourceID),
			slog.String("scope", st.Scope),
			slog.String("client_ip", ratelimit.ClientIP(r)))
		http.Error(w, `{"message":"forbidden"}`, http.StatusForbidden)
		return false
	}
	return true
}
