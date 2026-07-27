// Builds the authorization metadata a download must carry so canPlayOffline()
// (offlineAccess.ts) can later judge a replay by a DIFFERENT, more restricted
// profile. Extracted as one pure function shared by every screen that starts a
// download (media detail, music tracks/albums) instead of each screen
// inlining its own field mapping - a mismatch there would silently reopen the
// gap this whole feature closes.
//
// Pure: takes the already-resolved active profile and content classification
// (both of which each screen derives from data it already has in hand) and
// returns a plain object - no store reads, no I/O, safe to unit test and to
// call from a render path.
//
// Deliberately its own minimal shape rather than importing OfflineProfileInfo
// from offlineAccess.ts: only id/isKids/maxAge are needed to build the
// downloadedBy* fields (region only matters later, to canPlayOffline's rating
// comparison), and V1Profile's id is an optional property - matching that
// shape here (id?: ...) avoids a spurious required/optional mismatch at every
// call site that passes a V1Profile straight through.
export interface DownloaderProfile {
  id?: string | null;
  isKids?: boolean;
  maxAge?: number;
}

export interface DownloadableContent {
  officialRating?: string;
  certificationsByCountry?: Record<string, string>;
}

export interface DownloadAuthMetadata {
  downloadedByProfileId?: string;
  downloadedByIsKids?: boolean;
  downloadedByMaxAge?: number;
  officialRating?: string;
  certificationsByCountry?: Record<string, string>;
}

export function buildDownloadAuthContext(
  activeProfile: DownloaderProfile | null | undefined,
  content: DownloadableContent | null | undefined,
): DownloadAuthMetadata {
  return {
    downloadedByProfileId: activeProfile?.id ?? undefined,
    downloadedByIsKids: activeProfile?.isKids,
    downloadedByMaxAge: activeProfile?.maxAge,
    officialRating: content?.officialRating,
    certificationsByCountry: content?.certificationsByCountry,
  };
}
