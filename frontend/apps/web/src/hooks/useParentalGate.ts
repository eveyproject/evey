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

// Fail CLOSED on an unclassified or unresolvable rating, mirroring the server
// (ratings.WorstCaseMinAge < 0 blocks the item): a gated profile must never be
// offered content the backend is going to refuse, and unknown content is the
// one case where guessing wrong exposes a child. maxAge < 0 means the profile
// carries no age cap at all, so nothing is gated.
function isAgeExceeded(country: string, rating: string | undefined, maxAge: number): boolean {
  if (maxAge < 0) return false;
  if (!rating) return true;
  const age = minAgeFor(country, rating);
  if (age < 0) return true;
  return age > maxAge;
}

const SESSION_KEY = "evey:parental_unlocked";

function getUnlockedProfiles(): Set<string> {
  try {
    const raw = sessionStorage.getItem(SESSION_KEY);
    if (!raw) return new Set();
    return new Set(JSON.parse(raw) as string[]);
  } catch {
    return new Set();
  }
}

function markProfileUnlocked(profileId: string) {
  const set = getUnlockedProfiles();
  set.add(profileId);
  sessionStorage.setItem(SESSION_KEY, JSON.stringify([...set]));
}

export function isProfileUnlocked(profileId: string): boolean {
  return getUnlockedProfiles().has(profileId);
}

export function markUnlocked(profileId: string) {
  markProfileUnlocked(profileId);
}

export function useParentalSettings(profileId: string | null) {
  return useParentalServiceGetParentalSettings(profileId ?? "", {
    query: { enabled: !!profileId, staleTime: 60_000 },
  });
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

  function needsPin(): boolean {
    if (!profileId || !parental) return false;
    if (!parental.enabled) return false;
    if (!parental.hasPin) return false;
    if (isProfileUnlocked(profileId)) return false;
    const maxAge = parental.maxAge ?? -1;
    if (maxAge < 0) return false;
    const region = (profile?.region ?? "").toUpperCase();
    const rating = (region && certificationsByCountry?.[region]) || officialRating || "";
    return isAgeExceeded(region || "US", rating, maxAge);
  }

  return { needsPin, profileId };
}
