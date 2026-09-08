// Where the "auto-play the next episode" choice lives on the profile, so it
// follows the viewer from the web to the phone to the television. Stored in
// the profile's free-form preferences map like the other playback choices
// (allow_direct_play, subtitle_burn_in): a string, "true" or "false".
export const AUTOPLAY_NEXT_PREF_KEY = "autoplay_next_episode";

/** Whether the next episode should start on its own. Absent = on, the
 * behaviour every player had before the choice existed. */
export function autoplayNextEpisode(prefs: Record<string, string> | undefined): boolean {
  return prefs?.[AUTOPLAY_NEXT_PREF_KEY] !== "false";
}

/**
 * The preferences map to send when saving the choice.
 *
 * MERGES on purpose: UpdateProfile replaces the whole map server-side, so
 * sending just this key would wipe every other stored preference.
 */
export function autoplayNextPreferences(
  current: Record<string, string> | undefined,
  enabled: boolean,
): Record<string, string> {
  return { ...current, [AUTOPLAY_NEXT_PREF_KEY]: enabled ? "true" : "false" };
}
