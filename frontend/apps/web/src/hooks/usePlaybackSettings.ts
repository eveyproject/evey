import { usePlaybackSettingsServiceGetPlaybackSettings } from "@evey/api";

interface PlaybackSettings {
  skipForwardSeconds: number;
  skipBackwardSeconds: number;
}

const DEFAULTS: PlaybackSettings = { skipForwardSeconds: 30, skipBackwardSeconds: 30 };

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
