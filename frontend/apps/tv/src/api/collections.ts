import { useQuery, useInfiniteQuery } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";

export interface Collection {
  id: string;
  tmdbId?: number;
  name: string;
  overview?: string;
  posterUrl?: string;
  backdropUrl?: string;
  rules?: { field: string; operator: string; value: string }[];
  sortBy?: string;
  /** Members THIS viewer can see - already access- and parental-filtered. */
  itemCount?: number;
  /** "saga" (TMDB-sourced), "smart" (rule-driven) or "manual". */
  kind?: string;
  createdAt: string;
  updatedAt: string;
}

export interface CollectionItem {
  id: string;
  title: string;
  type: string;
  year?: number;
  overview?: string;
  communityRating?: number;
  genres?: string[];
}

interface ListCollectionsResponse {
  collections: Collection[];
}

interface GetCollectionResponse {
  collection: Collection;
  items: CollectionItem[];
  total: number;
}

export function useCollections() {
  return useQuery<ListCollectionsResponse>({
    queryKey: ["collections"],
    queryFn: () =>
      eveyFetch<ListCollectionsResponse>({
        url: "/api/v1/collections",
        method: "GET",
      }),
    staleTime: 5 * 60_000,
  });
}

const COLLECTION_PAGE_SIZE = 50;

/**
 * A collection and its members, page by page. The endpoint answers 50 at a
 * time, and one page showed 50 titles of a larger collection with nothing
 * saying the rest existed.
 */
export function useCollection(id: string, pageSize = COLLECTION_PAGE_SIZE) {
  const query = useInfiniteQuery<GetCollectionResponse>({
    queryKey: ["collection", id, pageSize],
    initialPageParam: 0,
    queryFn: ({ pageParam }) =>
      eveyFetch<GetCollectionResponse>({
        url: `/api/v1/collections/${id}?limit=${pageSize}&offset=${pageParam as number}`,
        method: "GET",
      }),
    getNextPageParam: (_last, pages) => {
      const loaded = pages.reduce((n, p) => n + (p.items?.length ?? 0), 0);
      return loaded < (pages[0]?.total ?? 0) ? loaded : undefined;
    },
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
  const pages = query.data?.pages ?? [];
  return {
    ...query,
    data: pages.length
      ? {
          collection: pages[0].collection,
          items: pages.flatMap((p) => p.items ?? []),
          total: pages[0].total ?? 0,
        }
      : undefined,
  };
}

/**
 * The sagas a title belongs to, already access- and parental-filtered by the
 * server. An empty list is the common case - a title belonging to nothing.
 */
export function useMediaItemCollections(mediaItemId: string) {
  return useQuery<ListCollectionsResponse>({
    queryKey: ["media-item-collections", mediaItemId],
    queryFn: () =>
      eveyFetch<ListCollectionsResponse>({
        url: `/api/v1/media/${mediaItemId}/collections`,
        method: "GET",
      }),
    enabled: !!mediaItemId,
    staleTime: 5 * 60_000,
  });
}
