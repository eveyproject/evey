import { useQuery } from "@tanstack/react-query";
import { useAuthStore } from "@evey/store";

export interface PlaybackSettings {
  skipForwardSeconds: number;
  skipBackwardSeconds: number;
}

const DEFAULTS: PlaybackSettings = { skipForwardSeconds: 10, skipBackwardSeconds: 10 };

export function usePlaybackSettings() {
  const token = useAuthStore((s) => s.accessToken);
  return useQuery<PlaybackSettings>({
    queryKey: ["playback-settings"],
    queryFn: async () => {
      const res = await fetch("/api/v1/playback-settings", {
        headers: token ? { Authorization: `Bearer ${token}` } : {},
      });
      if (!res.ok) return DEFAULTS;
      return res.json() as Promise<PlaybackSettings>;
    },
    staleTime: 5 * 60 * 1000,
    select: (d) => ({
      skipForwardSeconds: d.skipForwardSeconds ?? DEFAULTS.skipForwardSeconds,
      skipBackwardSeconds: d.skipBackwardSeconds ?? DEFAULTS.skipBackwardSeconds,
    }),
  });
}
