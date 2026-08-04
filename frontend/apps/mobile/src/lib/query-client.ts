import { QueryClient } from "@tanstack/react-query";
import { setupOnlineManager } from "./online-manager";
import { mutationErrorNet } from "./mutationErrorNet";

// Wired here rather than in a component so it is installed exactly once, before
// the first query is ever created. Without it React Query never learns the
// device went offline (it listens for a browser `online` event React Native
// does not emit), so refetchOnReconnect is dead and a tunnel leaves every
// screen stale.
setupOnlineManager();

export const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      // Don't retry errors a retry cannot fix. `retry: 2` used to burn two
      // backoffs on a deleted item (404) or a forbidden one (403) before the
      // screen could react; the fetcher attaches httpStatus on every non-2xx.
      // 401 is handled by the refresh flow, not here.
      retry: (count, error) => {
        const status = (error as { httpStatus?: number }).httpStatus;
        if (status === 401 || status === 403 || status === 404) return false;
        return count < 2;
      },
    },
  },
});
