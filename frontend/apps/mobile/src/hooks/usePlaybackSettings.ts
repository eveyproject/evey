import { usePlaybackSettingsServiceGetPlaybackSettings } from "@evey/api";

export interface PlaybackSettings {
  skipForwardSeconds: number;
  skipBackwardSeconds: number;
}

const DEFAULTS: PlaybackSettings = { skipForwardSeconds: 30, skipBackwardSeconds: 30 };

// Server-wide skip-forward/backward seconds (admin-editable, defaults 30).
// Mirrors the web hook so the mobile player honours the same setting instead of
// hardcoding a step.
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
