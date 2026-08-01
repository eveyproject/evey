import { useQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

// Mirror of mobile's src/hooks/usePopular.ts (and of web's @/hooks/usePopular):
// the "most popular" home row is the same endpoint everywhere, so the three
// clients must not each invent their own shape for it.
interface PopularItem {
  mediaItemId: string;
  title: string;
  type?: string;
  playCount: number;
}

export function useMostPopular(limit = 20) {
  return useQuery({
    queryKey: ["popular", limit],
    queryFn: () =>
      eveyFetch<{ items: PopularItem[] }>({
        url: "/api/v1/popular",
        method: "GET",
      }),
    select: (d) => d.items ?? [],
    staleTime: 5 * 60_000,
  });
}
