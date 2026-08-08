import { useQuery } from "@tanstack/react-query";
import { eveyFetch, useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";

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

interface ParentalSettings {
  maxAge?: number;
  hasPin: boolean;
  enabled: boolean;
}

const unlockedProfiles = new Set<string>();

export function isProfileUnlocked(profileId: string): boolean {
  return unlockedProfiles.has(profileId);
}

export function markUnlocked(profileId: string) {
  unlockedProfiles.add(profileId);
}

function useParentalSettings(profileId: string | null) {
  return useQuery<ParentalSettings>({
    queryKey: [`/api/v1/profiles/${profileId}/parental`],
    queryFn: () =>
      eveyFetch<ParentalSettings>({
        url: `/api/v1/profiles/${profileId}/parental`,
        method: "GET",
      }),
    enabled: !!profileId,
    staleTime: 60_000,
  });
}

// pinRequiredFor is the ONE PIN decision every play path shares (Play button,
// episode row, shuffle). Pure so callers whose rating varies per pick
// (shuffle draws a random item) can evaluate it outside the hook.
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
  const { data: parental } = useParentalSettings(profileId);
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } } as never);
  const profile = profilesResp?.profiles?.find((p) => p.id === profileId);
  const region = profile?.region ?? "";

  function needsPinFor(rating: string | undefined, certs?: Record<string, string>): boolean {
    return pinRequiredFor({ profileId, parental, region, rating, certificationsByCountry: certs });
  }

  function needsPin(): boolean {
    return needsPinFor(officialRating, certificationsByCountry);
  }

  return { needsPin, needsPinFor, profileId };
}
