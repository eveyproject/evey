import { QueryClient } from "@tanstack/react-query";
import { shouldRetryQuery } from "@evey/store";
import { mutationErrorNet } from "./mutationErrorNet";

export const queryClient = new QueryClient({
  mutationCache: mutationErrorNet(),
  defaultOptions: {
    queries: {
      staleTime: 5 * 60 * 1000,
      // Shared with web and mobile: see @evey/store/queryRetry.
      retry: shouldRetryQuery,
    },
  },
});
