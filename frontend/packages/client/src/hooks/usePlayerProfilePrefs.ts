import { useCallback, useEffect, useMemo, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getUserServiceListProfilesQueryKey, useUserServiceUpdateProfile, type V1Profile } from "@evey/api";
import { autoplayNextEpisode, autoplayNextPreferences } from "@evey/store";
import { useActiveProfile } from "./useActiveProfile";
import { setStillWatchingLimit, stillWatchingLimit } from "../lib/stillWatching";

/** A profile preference stored as a comma-separated list ("fr,en"). */
export function profileLangList(raw: string | undefined): string[] {
  return (raw ?? "").split(",").map((s) => s.trim()).filter(Boolean);
}

/**
 * What every player reads from the running profile: its ordered audio and
 * subtitle languages, and its "auto-play next episode" choice with the toggle
 * the player's own menu flips. The toggle is optimistic and persisted on the
 * profile, so the choice follows the viewer from one client to the next; a
 * failed save falls back to the server value (the global mutation handler
 * already reported it).
 */
export function usePlayerProfilePrefs(): {
  profile: V1Profile | undefined;
  audioLangs: string[];
  subtitleLangs: string[];
  autoplayNext: boolean;
  setAutoplayNext: (enabled: boolean) => Promise<void>;
} {
  const { profile, userId } = useActiveProfile();
  const qc = useQueryClient();
  const updateProfile = useUserServiceUpdateProfile();
  const audioLangs = useMemo(() => profileLangList(profile?.preferences?.["audio_languages"]), [profile]);
  const subtitleLangs = useMemo(() => profileLangList(profile?.preferences?.["subtitle_languages"]), [profile]);

  const [override, setOverride] = useState<boolean | null>(null);
  // The refetched profile is the truth again once it lands.
  useEffect(() => { setOverride(null); }, [profile?.preferences]);
  const autoplayNext = override ?? autoplayNextEpisode(profile?.preferences);
  // The running profile's "still watching?" threshold drives the shared gate.
  const stillWatchingAfter = stillWatchingLimit(profile?.preferences);
  useEffect(() => { setStillWatchingLimit(stillWatchingAfter); }, [stillWatchingAfter]);
  const setAutoplayNext = useCallback(async (enabled: boolean) => {
    if (!profile?.id) return;
    setOverride(enabled);
    try {
      await updateProfile.mutateAsync({
        id: profile.id,
        data: { preferences: autoplayNextPreferences(profile.preferences, enabled) },
      });
      if (userId) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
    } catch {
      setOverride(null);
    }
  }, [profile, updateProfile, qc, userId]);

  return { profile, audioLangs, subtitleLangs, autoplayNext, setAutoplayNext };
}
