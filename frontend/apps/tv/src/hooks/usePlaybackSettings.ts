import { usePlaybackSettingsServiceGetPlaybackSettings } from "@evey/api";

interface PlaybackSettings {
  skipForwardSeconds: number;
  skipBackwardSeconds: number;
}

const DEFAULTS: PlaybackSettings = { skipForwardSeconds: 30, skipBackwardSeconds: 30 };

// Server-wide skip-forward/backward seconds (admin-editable, defaults 30).
// Mirrors the web/mobile hook so the D-pad left/right skip honours the same
// setting instead of a hardcoded step.
export function usePlaybackSettings() {
  return usePlaybackSettingsServiceGetPlaybackSettings({
    query: {
      staleTime: 5 * 60 * 1000,
      select: (d) => ({
        skipForwardSeconds: d.skipForwardSeconds ?? DEFAULTS.skipForwardSeconds,
        skipBackwardSeconds: d.skipBackwardSeconds ?? DEFAULTS.skipBackwardSeconds,
      }),
    },
  });
}
