import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { getUserServiceListProfilesQueryKey, useUserServiceUpdateProfile, type V1Profile } from "@evey/api";
import { autoplayNextEpisode, autoplayNextPreferences, isDemoMode } from "@evey/store";
import { useActiveProfile } from "./useActiveProfile";
import { setStillWatchingLimit, stillWatchingLimit } from "../lib/stillWatching";
import { parseSubtitleStylePref, subtitleStylePreferences, SUBTITLE_STYLE_PREF_KEY, type SubtitleStyle } from "../lib/subtitleStyle";

// A size slider fires on every step: one save once the viewer stops.
const SUBTITLE_STYLE_SAVE_DELAY_MS = 600;

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
 *
 * The subtitle appearance follows the profile the same way: subtitleStyle is
 * what the profile holds (null when it never chose, the player then keeps its
 * own last value), saveSubtitleStyle stores a change on it.
 */
export function usePlayerProfilePrefs(): {
  profile: V1Profile | undefined;
  audioLangs: string[];
  subtitleLangs: string[];
  autoplayNext: boolean;
  setAutoplayNext: (enabled: boolean) => Promise<void>;
  subtitleStyle: SubtitleStyle | null;
  saveSubtitleStyle: (style: SubtitleStyle) => void;
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

  // The profile's value, unless a change made here is still on its way to the
  // server: a reload landing in between must not put the previous one back.
  const storedStyleRaw = profile?.preferences?.[SUBTITLE_STYLE_PREF_KEY];
  const storedStyle = useMemo(
    () => parseSubtitleStylePref(storedStyleRaw ? { [SUBTITLE_STYLE_PREF_KEY]: storedStyleRaw } : undefined),
    [storedStyleRaw],
  );
  const [pendingStyle, setPendingStyle] = useState<SubtitleStyle | null>(null);
  const saveTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const latest = useRef({ profile, updateProfile, qc, userId });
  latest.current = { profile, updateProfile, qc, userId };
  // The change still waiting for its timer, and whether the screen is still up.
  const unsavedStyle = useRef<SubtitleStyle | null>(null);
  const mounted = useRef(true);
  const sendStyle = useCallback(async (style: SubtitleStyle) => {
    const { profile: p, updateProfile: update, qc: client, userId: uid } = latest.current;
    try {
      if (!p?.id) return;
      await update.mutateAsync({ id: p.id, data: { preferences: subtitleStylePreferences(p.preferences, style) } });
      if (uid) await client.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(uid) });
    } catch {
      // The global mutation handler already reported it; the profile's value stands.
    } finally {
      // A newer change restarted the timer: it owns the pending value now.
      if (mounted.current && !saveTimer.current) setPendingStyle(null);
    }
  }, []);
  // Leaving the screen inside the delay sends the last change at once rather
  // than dropping it: the viewer picked it, the next title must open with it.
  useEffect(() => {
    mounted.current = true;
    return () => {
      mounted.current = false;
      if (!saveTimer.current) return;
      clearTimeout(saveTimer.current);
      saveTimer.current = null;
      const style = unsavedStyle.current;
      unsavedStyle.current = null;
      if (style) void sendStyle(style);
    };
  }, [sendStyle]);
  const saveSubtitleStyle = useCallback((style: SubtitleStyle) => {
    // Demo: the shared profile's appearance is not this visitor's pick.
    if (isDemoMode()) return;
    setPendingStyle(style);
    unsavedStyle.current = style;
    if (saveTimer.current) clearTimeout(saveTimer.current);
    saveTimer.current = setTimeout(() => {
      saveTimer.current = null;
      unsavedStyle.current = null;
      void sendStyle(style);
    }, SUBTITLE_STYLE_SAVE_DELAY_MS);
  }, [sendStyle]);
  const subtitleStyle = pendingStyle ?? storedStyle;

  return { profile, audioLangs, subtitleLangs, autoplayNext, setAutoplayNext, subtitleStyle, saveSubtitleStyle };
}
