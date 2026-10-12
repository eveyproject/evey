package httpapi

import (
	"errors"
	"net/http"

	appmedia "github.com/eveyproject/evey/internal/app/media"
	"github.com/eveyproject/evey/internal/domain/library"
)

// shareDownloadHandler serves a media file, or a season/series ZIP, via a
// public share token - NO auth.
// The token (a uuid) IS the capability: an admin created the link for a media
// they had access to, and the token entropy is the access control. Validate()
// enforces expiry + view-limit and increments the view count. This is the only
// public-facing use of share links; creation/listing/revoke are admin-gated.
func shareDownloadHandler(shareSvc *appmedia.ShareService, libSvc MediaArchiveSource, baseURL string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.PathValue("token")
		if token == "" {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		link, err := shareSvc.Validate(r.Context(), token)
		if err != nil {
			if errors.Is(err, appmedia.ErrShareExpired) || errors.Is(err, appmedia.ErrShareViewLimit) {
				http.Error(w, `{"message":"link expired or view limit reached"}`, http.StatusGone)
				return
			}
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		item, err := libSvc.GetMediaItem(r.Context(), string(link.MediaItemID))
		if err != nil || item == nil {
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
			return
		}
		if item.File != nil && item.File.Path != "" {
			serveDownload(w, r, item, baseURL, mediaRoots(r.Context(), libSvc))
			return
		}
		// A shared season or series is the same ZIP an authenticated download
		// gets. No profile to gate on: the admin chose what to share.
		allowAll := func(*library.MediaItem) bool { return true }
		switch item.Type {
		case library.Season:
			serveSeasonArchive(w, r, libSvc, item, baseURL, mediaRoots(r.Context(), libSvc), allowAll)
		case library.Series:
			serveSeriesArchive(w, r, libSvc, item, baseURL, mediaRoots(r.Context(), libSvc), allowAll)
		default:
			http.Error(w, `{"message":"not found"}`, http.StatusNotFound)
		}
	}
}
