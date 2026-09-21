import { useParentalServiceGetParentalSettings, useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";

// Minimal per-country rating → min-age table. Mirrors the backend
// internal/pkg/ratings byCountry map for the handful of systems TMDB returns
// most often for movies/series.
const BY_COUNTRY: Record<string, Record<string, number>> = {
  FR: { TP: 0, U: 0, "-10": 10, "10": 10, "-12": 12, "12": 12, "-16": 16, "16": 16, "-18": 18, "18": 18, X: 18 },
  US: { G: 0, "TV-Y": 0, "TV-G": 0, "TV-Y7": 7, PG: 10, "TV-PG": 10, "PG-13": 13, "TV-14": 14, R: 17, "TV-MA": 17, "NC-17": 17 },
  GB: { U: 0, UC: 0, PG: 8, "12": 12, "12A": 12, "15": 15, "18": 18, R18: 18 },
  DE: { "0": 0, "6": 6, "12": 12, "16": 16, "18": 18 },
  ES: { APTA: 0, TP: 0, "7": 7, "12": 12, "13": 13, "16": 16, "18": 18 },
  AU: { G: 0, PG: 8, M: 15, "MA 15+": 15, "MA15+": 15, "R 18+": 18, "R18+": 18 },
  BR: { L: 0, LIVRE: 0, "10": 10, "12": 12, "14": 14, "16": 16, "18": 18 },
  NL: { AL: 0, "6": 6, "9": 9, "12": 12, "14": 14, "16": 16, "18": 18 },
  RU: { "0+": 0, "6+": 6, "12+": 12, "14+": 14, "16+": 16, "18+": 18 },
  IT: { T: 0, "6+": 6, "12": 12, "14": 14, "18": 18 },
  JP: { G: 0, PG12: 12, "R15+": 15, "R18+": 18 },
  IN: { U: 0, "U/A": 12, "U/A 7+": 7, "U/A 13+": 13, "U/A 16+": 16, A: 18 },
};

function minAgeFor(country: string, rating: string): number {
  if (!country || !rating) return -1;
  const c = country.toUpperCase();
  const r = rating.trim().toUpperCase();
  if (BY_COUNTRY[c]?.[r] !== undefined) return BY_COUNTRY[c][r];
  if (c !== "US" && BY_COUNTRY.US[r] !== undefined) return BY_COUNTRY.US[r];
  return -1;
}

// The SERVER is authoritative and already fails closed: it computes the
// worst-case minimum age across every country's certification and refuses the
// item (404, list-filtered) when that age exceeds the profile's cap or cannot
// be resolved. Anything this client can see has therefore been vetted for the
// active profile.
//
// So this gate must never be STRICTER than the server: it holds a 12-country
// table where the server holds 26, and it only sees officialRating (often
// empty on titles that carry per-country certifications). Failing closed here
// would demand a parental PIN for legitimate, all-ages content - verified on a
// real title rated TP in France whose officialRating is empty. Unresolvable
// rating -> defer to the server, which already decided.
export function isAgeExceeded(country: string, rating: string | undefined, maxAge: number): boolean {
  if (!rating || maxAge < 0) return false;
  const age = minAgeFor(country, rating);
  if (age < 0) return false;
  return age > maxAge;
}

// --- Unlock registry ---------------------------------------------------------
//
// Which profiles have entered their PIN this session. The POLICY is the
// platform's, not this module's: the web keeps it in sessionStorage so a
// reload does not ask again, a phone keeps it in memory for the process, a
// television forgets it after a while because the remote is shared by the
// whole living room. Each app installs its policy once at startup through
// configureParentalUnlock(); the default is "in memory, for the process".

export interface ParentalUnlockStore {
  /** Profile ids currently unlocked. */
  load(): Set<string>;
  /** Persist the full set (the registry never writes one id at a time). */
  save(ids: Set<string>): void;
}

export interface ParentalUnlockOptions {
  /** Where the unlocked set lives. Default: this process's memory. */
  store?: ParentalUnlockStore;
  /** An unlock expires this long after it was granted. Default: never. */
  ttlMs?: number;
  /** Clock, overridable for tests. */
  now?: () => number;
}

function memoryStore(): ParentalUnlockStore {
  let ids = new Set<string>();
  return {
    load: () => new Set(ids),
    save: (next) => {
      ids = new Set(next);
    },
  };
}

let unlockStore: ParentalUnlockStore = memoryStore();
let unlockTtlMs: number | undefined;
let unlockNow: () => number = () => Date.now();
// Grant times, kept beside the store so a persisted store only ever holds ids.
const grantedAt = new Map<string, number>();

export function configureParentalUnlock(options: ParentalUnlockOptions): void {
  unlockStore = options.store ?? memoryStore();
  unlockTtlMs = options.ttlMs;
  unlockNow = options.now ?? (() => Date.now());
  grantedAt.clear();
}

export function isProfileUnlocked(profileId: string): boolean {
  const ids = unlockStore.load();
  if (!ids.has(profileId)) return false;
  if (unlockTtlMs !== undefined) {
    const at = grantedAt.get(profileId);
    // An id persisted without a grant time (store restored from outside this
    // process) is treated as freshly granted rather than as expired.
    if (at === undefined) grantedAt.set(profileId, unlockNow());
    else if (unlockNow() - at > unlockTtlMs) {
      ids.delete(profileId);
      grantedAt.delete(profileId);
      unlockStore.save(ids);
      return false;
    }
  }
  return true;
}

export function markUnlocked(profileId: string): void {
  const ids = unlockStore.load();
  ids.add(profileId);
  grantedAt.set(profileId, unlockNow());
  unlockStore.save(ids);
}

/** Forget every unlock - what a profile switch or a sign-out calls. */
export function clearUnlocked(): void {
  grantedAt.clear();
  unlockStore.save(new Set());
}

// --- Settings + gate ---------------------------------------------------------

export interface ParentalSettings {
  maxAge?: number;
  hasPin?: boolean;
  enabled?: boolean;
}

function useParentalSettings(profileId: string | null) {
  return useParentalServiceGetParentalSettings(profileId ?? "", {
    query: { enabled: !!profileId, staleTime: 60_000 },
  });
}

/**
 * The gate decision as a pure function, so a screen that draws many titles
 * (a library grid) can ask per card without a hook per card.
 */
export function pinRequiredFor(args: {
  profileId: string | null;
  parental: ParentalSettings | undefined;
  region: string;
  rating: string | undefined;
  certificationsByCountry?: Record<string, string>;
}): boolean {
  const { profileId, parental, certificationsByCountry } = args;
  if (!profileId || !parental) return false;
  if (!parental.enabled) return false;
  if (!parental.hasPin) return false;
  if (isProfileUnlocked(profileId)) return false;
  const maxAge = parental.maxAge ?? -1;
  if (maxAge < 0) return false;
  const region = args.region.toUpperCase();
  const rating = (region && certificationsByCountry?.[region]) || args.rating || "";
  return isAgeExceeded(region || "US", rating, maxAge);
}

export function useParentalGate(
  officialRating: string | undefined,
  certificationsByCountry?: Record<string, string>,
) {
  const profileId = useAuthStore((s) => s.profileId);
  const { data: parental, isLoading: settingsLoading } = useParentalSettings(profileId);
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const profile = profilesResp?.profiles?.find((p) => p.id === profileId);
  const region = profile?.region ?? "";

  function needsPinFor(rating: string | undefined, certs?: Record<string, string>): boolean {
    return pinRequiredFor({ profileId, parental, region, rating, certificationsByCountry: certs });
  }

  function needsPin(): boolean {
    return needsPinFor(officialRating, certificationsByCountry);
  }

  // settingsLoading: the caller must NOT trust needsPin() yet - the settings
  // are still in flight, and answering "false" during that window let the
  // player mount and start a transcode session before the PIN dialog showed.
  // isLoading (pending AND fetching) is deliberately used over isPending: a
  // disabled query (no profile) stays pending forever but never fetches, and
  // the gate must not hang on it. A failed fetch also ends loading and falls
  // open - this gate is UI comfort, the server re-verifies every request.
  return { needsPin, needsPinFor, profileId, settingsLoading: !!profileId && settingsLoading };
}
