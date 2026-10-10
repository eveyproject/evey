import { useQuery } from "@tanstack/react-query";
import { useServerUrlNullable } from "./useServerUrl";
import { newerMobileRelease, type MobileRelease } from "../lib/appUpdate";

// Asked on start and every few hours: the app lives in the background for
// days, and a release published meanwhile should reach the banner without
// a cold start.
const CHECK_EVERY_MS = 6 * 60 * 60 * 1000;

// The newer phone build the server was released with, or null. A failed
// check offers nothing: a missing banner is the honest degradation.
export function useAppUpdate(): MobileRelease | null {
  const serverUrl = useServerUrlNullable();
  const { data } = useQuery({
    queryKey: ["mobile-app-update", serverUrl],
    enabled: !!serverUrl,
    staleTime: CHECK_EVERY_MS,
    refetchInterval: CHECK_EVERY_MS,
    retry: false,
    queryFn: async () => {
      const res = await fetch(`${serverUrl}/healthz`);
      return newerMobileRelease(await res.json());
    },
  });
  return data ?? null;
}
