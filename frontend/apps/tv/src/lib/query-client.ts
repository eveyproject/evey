import { QueryClient } from "@tanstack/react-query";
import { mutationErrorNet } from "./mutationErrorNet";

export const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      // Don't retry errors a retry cannot fix: 401 (auth, handled by the
      // refresh flow), 403 (forbidden) and 404 (gone). Retrying blindly made
      // the remote wait through two backoffs for an outcome that could never
      // change, tripling the load on the server for nothing. Mirrors web
      // (src/main.tsx) and mobile (src/lib/query-client.ts).
      retry: (count, error) => {
        const status = (error as { httpStatus?: number }).httpStatus;
        if (status === 401 || status === 403 || status === 404) return false;
        return count < 2;
      },
    },
  },
});
