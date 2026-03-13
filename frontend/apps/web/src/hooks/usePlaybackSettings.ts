import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface PlaybackSettings {
  skipForwardSeconds: number;
  skipBackwardSeconds: number;
}

const DEFAULTS: PlaybackSettings = { skipForwardSeconds: 10, skipBackwardSeconds: 10 };

export function usePlaybackSettings() {
  return useQuery<PlaybackSettings>({
    queryKey: ["playback-settings"],
    queryFn: async () => {
      try {
        return await eveyFetch<PlaybackSettings>({ url: "/api/v1/playback-settings", method: "GET" });
      } catch {
        return DEFAULTS;
      }
    },
    staleTime: 5 * 60 * 1000,
    select: (d) => ({
      skipForwardSeconds: d.skipForwardSeconds ?? DEFAULTS.skipForwardSeconds,
      skipBackwardSeconds: d.skipBackwardSeconds ?? DEFAULTS.skipBackwardSeconds,
    }),
  });
}
