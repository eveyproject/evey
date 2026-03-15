import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { useAuthStore } from "@evey/store";

const RATING_SEVERITY: Record<string, number> = {
  G: 0, "TV-Y": 0, "TV-Y7": 0, "TV-G": 0,
  PG: 1, "TV-PG": 1,
  "PG-13": 2, "TV-14": 2,
  R: 3, "TV-MA": 3,
  "NC-17": 4,
};

function isRatingExceeded(contentRating: string | undefined, maxRating: string): boolean {
  if (!contentRating || !maxRating) return false;
  const content = contentRating.trim().toUpperCase();
  const max = maxRating.trim().toUpperCase();
  const contentLevel = RATING_SEVERITY[content];
  const maxLevel = RATING_SEVERITY[max];
  if (contentLevel === undefined || maxLevel === undefined) return false;
  return contentLevel > maxLevel;
}

interface ParentalSettings {
  maxRating: string;
  hasPin: boolean;
  enabled: boolean;
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

export function useParentalGate(officialRating: string | undefined) {
  const profileId = useAuthStore((s) => s.profileId);
  const { data: parental } = useParentalSettings(profileId);

  function needsPin(): boolean {
    if (!profileId || !parental) return false;
    if (!parental.enabled) return false;
    if (!parental.hasPin) return false;
    if (isProfileUnlocked(profileId)) return false;
    return isRatingExceeded(officialRating, parental.maxRating);
  }

  return { needsPin, profileId };
}
