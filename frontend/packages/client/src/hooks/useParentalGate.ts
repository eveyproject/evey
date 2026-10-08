import { useParentalServiceGetParentalSettings, useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { RATINGS_BY_COUNTRY } from "./ratingsTable";

// Per-country rating -> min-age table: the server's byCountry (internal/
// domain/ratings), rendered into ratingsTable.ts by a server test that fails
// when the two drift. It used to be a hand-kept 12-country subset, so a title
// the server gated on a Canadian or Portuguese certificate played here
// without the PIN prompt.
const BY_COUNTRY: Record<string, Record<string, number>> = RATINGS_BY_COUNTRY;

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
// So this gate must never be STRICTER than the server: it holds the server's
// table but only sees officialRating (often empty on titles that carry
// per-country certifications), and it does not strip agency prefixes
// ("ACB MA 15+") the way the server does. Failing closed here
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
