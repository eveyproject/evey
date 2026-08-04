import { useQuery, useInfiniteQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import i18n from "../i18n";

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

/**
 * Every collection, including the auto-created sagas the browse listing hides
 * for holding a single title. A curation surface must show them: hiding the
 * saga you own one film of is right when browsing and wrong when the user is
 * trying to add the second one to it.
 */
export function useAllCollections() {
  return useQuery<ListCollectionsResponse>({
    queryKey: ["collections", "all"],
    queryFn: () =>
      eveyFetch<ListCollectionsResponse>({
        url: "/api/v1/collections?minItems=1",
        method: "GET",
      }),
    staleTime: 5 * 60_000,
  });
}

const COLLECTION_PAGE_SIZE = 50;

/**
 * A collection and its members, page by page. It has to page: the endpoint
 * answers 50 members at a time, and asking for one page showed 50 titles of a
 * larger collection with nothing saying the rest existed.
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

export function useCreateCollection() {
  const qc = useQueryClient();
  return useMutation({
    // The server field is overview, not description: what was sent under the
    // old name was dropped on the floor, so a description typed at creation
    // never survived the save.
    mutationFn: (input: { name: string; overview?: string }) =>
      eveyFetch<{ id: string }>({ url: "/api/v1/collections", method: "POST", data: input }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      toast.success(i18n.t("media.collections.created"));
    },
    onError: () => toast.error(i18n.t("media.collections.createFailed")),
  });
}

export function useUpdateCollection() {
  const qc = useQueryClient();
  return useMutation({
    // overview is optional on the wire: leaving it out keeps what is stored,
    // which is how a rename spares the synopsis a TMDB saga came with.
    mutationFn: ({ id, ...body }: { id: string; name: string; overview?: string }) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "PUT", data: body }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      qc.invalidateQueries({ queryKey: ["collection", vars.id] });
      toast.success(i18n.t("media.collections.updated"));
    },
    onError: () => toast.error(i18n.t("media.collections.updateFailed")),
  });
}

export function useDeleteCollection() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "DELETE" }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      toast.success(i18n.t("media.collections.deleted"));
    },
    onError: () => toast.error(i18n.t("media.collections.deleteFailed")),
  });
}

/** Admin-only (RBAC RoleAdmin server-side); does not delete the title itself. */
export function useRemoveCollectionItem() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, mediaItemId }: { id: string; mediaItemId: string }) =>
      eveyFetch<void>({ url: `/api/v1/collections/${id}/items/${mediaItemId}`, method: "DELETE" }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      qc.invalidateQueries({ queryKey: ["collection", vars.id] });
      toast.success(i18n.t("media.collections.itemRemoved"));
    },
    onError: () => toast.error(i18n.t("common.error")),
  });
}

/**
 * Writes the hand-set order of a collection's members. Admin-only, mirrors
 * apps/web's useReorderCollectionItems (features/collection/api.ts).
 *
 * The order only takes effect once the collection reads by it, so this also
 * switches sortBy to "manual" in a second call - a viewer who moves a title
 * means that place, not "store it and keep showing release order".
 */
export function useReorderCollectionItems(id: string) {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: async ({ mediaItemIds, name }: { mediaItemIds: string[]; name: string }) => {
      await eveyFetch<void>({ url: `/api/v1/collections/${id}/items/order`, method: "PUT", data: { mediaItemIds } });
      await eveyFetch<void>({ url: `/api/v1/collections/${id}`, method: "PUT", data: { name, sortBy: "manual" } });
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["collections"] });
      qc.invalidateQueries({ queryKey: ["collection", id] });
    },
    onError: () => toast.error(i18n.t("common.error")),
  });
}
