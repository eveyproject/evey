// Decides whether a media file already downloaded to this device may still
// be played, given the profile currently active on the app.
//
// Rationale (security audit finding): a downloaded file used to be replayed
// with zero authorization re-check. This module is the local (offline)
// half of the fix - see getDownloadPath() in ./downloads for the online
// half (server revalidation via an existing generated call).
import { isAgeExceeded } from "../hooks/useParentalGate";
import type { DownloadedMedia } from "./downloads";

// How long a downloaded file stays playable without the app being able to
// re-check server-side authorization (library access, parental settings).
// Named so it is tuned in one place instead of a scattered magic number.
// 30 days mirrors the "offline license" window of other streaming apps and
// comfortably covers the "watch on a flight" use case this feature exists for.
export const OFFLINE_DOWNLOAD_TTL_MS = 30 * 24 * 60 * 60 * 1000;

// Minimal profile shape needed to decide offline playability - a subset of
// V1Profile (id/isKids/maxAge/region) so this module stays a pure function
// of its inputs instead of reaching into React Query caches or the auth
// store itself.
export interface OfflineProfileInfo {
  id: string | null | undefined;
  isKids?: boolean;
  maxAge?: number;
  region?: string;
  /**
   * Whether isKids/maxAge above actually describe the active profile, or are
   * simply absent because nobody could ask the server.
   *
   * The two used to be indistinguishable (`isKids: undefined` meant both "not
   * a kids profile" and "no idea"), and the second silently resolved to an
   * UNRESTRICTED cap. That was harmless while the app could not even be opened
   * without a server; it stopped being harmless the day a cold start with no
   * network became a supported way in, because the profile list is exactly
   * what never arrives then. Callers that resolved a real profile object pass
   * true; callers that only have a profile id pass false.
   */
  restrictionKnown?: boolean;
}

export type OfflineDenialReason = "unknown-profile" | "expired" | "age-gated" | "more-restricted-profile";

export interface OfflineDecision {
  allowed: boolean;
  reason?: OfflineDenialReason;
}

// A profile with isKids=true but no recorded maxAge uses the server's
// implicit kids default - mirrors the doc comment on V1Profile.maxAge.
const IMPLICIT_KIDS_MAX_AGE = 12;

// Effective age cap for a profile: Infinity (unrestricted) for a non-kids
// profile, else its own cap (or the implicit default above).
function restrictionCap(p: { isKids?: boolean; maxAge?: number } | undefined): number {
  if (!p?.isKids) return Infinity;
  return typeof p.maxAge === "number" && p.maxAge >= 0 ? p.maxAge : IMPLICIT_KIDS_MAX_AGE;
}

/**
 * Pure decision function - no I/O, no clock reads (now is a parameter),
 * safe to unit test exhaustively and safe to call from a render path.
 *
 * Decision table (checked in this order):
 *  1. Expired (now - downloadedAt > OFFLINE_DOWNLOAD_TTL_MS)      -> refuse "expired"
 *  2. No active profile identity at all                          -> refuse "unknown-profile"
 *  3. Active profile is age-gated and the content classification
 *     exceeds its cap                                            -> refuse "age-gated"
 *  4. Active profile differs from the one who downloaded it AND
 *     either its restriction is unknown, or it is more restricted
 *     than the downloader                                        -> refuse "more-restricted-profile"
 *  5. Otherwise                                                  -> allow
 *
 * Legacy entries that predate downloadedByProfileId (rule 4) or a rating
 * capture (rule 3) are never retroactively blocked by those two rules alone -
 * matching the existing useParentalGate philosophy of deferring to the
 * server when the client can't resolve enough to judge.
 */
export function canPlayOffline(
  entry: DownloadedMedia,
  activeProfile: OfflineProfileInfo | null | undefined,
  now: number,
): OfflineDecision {
  const downloadedAtMs = Date.parse(entry.downloadedAt);
  if (Number.isFinite(downloadedAtMs) && now - downloadedAtMs > OFFLINE_DOWNLOAD_TTL_MS) {
    return { allowed: false, reason: "expired" };
  }

  if (!activeProfile || !activeProfile.id) {
    return { allowed: false, reason: "unknown-profile" };
  }

  const activeCap = restrictionCap(activeProfile);
  if (activeCap < Infinity) {
    const region = (activeProfile.region ?? "").toUpperCase();
    const rating = (region && entry.certificationsByCountry?.[region]) || entry.officialRating || "";
    if (isAgeExceeded(region || "US", rating, activeCap)) {
      return { allowed: false, reason: "age-gated" };
    }
  }

  if (entry.downloadedByProfileId && entry.downloadedByProfileId !== activeProfile.id) {
    // Nobody could tell us what this profile is allowed to watch, so we cannot
    // clear ANOTHER profile's download for it: the unknown cap would resolve to
    // Infinity below and a kids profile would replay whatever an adult
    // downloaded on this device. Replaying your own download is unaffected,
    // which is the only case a legitimate offline viewer is in.
    if (activeProfile.restrictionKnown === false) {
      return { allowed: false, reason: "more-restricted-profile" };
    }
    const downloaderCap = restrictionCap({
      isKids: entry.downloadedByIsKids,
      maxAge: entry.downloadedByMaxAge,
    });
    if (activeCap < downloaderCap) {
      return { allowed: false, reason: "more-restricted-profile" };
    }
  }

  return { allowed: true };
}

export class OfflineAccessError extends Error {
  reason: OfflineDenialReason | "revoked";

  constructor(reason: OfflineDenialReason | "revoked", message: string) {
    super(message);
    this.name = "OfflineAccessError";
    this.reason = reason;
  }
}
