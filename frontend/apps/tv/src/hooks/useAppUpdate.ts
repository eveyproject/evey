import { useQuery } from "@tanstack/react-query";
import { useServerUrlNullable } from "./useServerUrl";
import { newerTVRelease, type TVRelease } from "../lib/appUpdate";

// Asked on start and every few hours: a TV stays on for days, and a release
// published meanwhile should reach the header without a restart.
const CHECK_EVERY_MS = 6 * 60 * 60 * 1000;

// The newer TV build the server was released with, or null. A failed check
// offers nothing: a missing banner is the honest degradation.
export function useAppUpdate(): TVRelease | null {
  const serverUrl = useServerUrlNullable();
  const { data } = useQuery({
    queryKey: ["tv-app-update", serverUrl],
    enabled: !!serverUrl,
    staleTime: CHECK_EVERY_MS,
    refetchInterval: CHECK_EVERY_MS,
    retry: false,
    queryFn: async () => {
      const res = await fetch(`${serverUrl}/healthz`);
      return newerTVRelease(await res.json());
    },
  });
  return data ?? null;
}
