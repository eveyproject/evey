import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
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

export function useCollection(id: string, limit = 50, offset = 0) {
  return useQuery<GetCollectionResponse>({
    queryKey: ["collection", id, limit, offset],
    queryFn: () =>
      eveyFetch<GetCollectionResponse>({
        url: `/api/v1/collections/${id}?limit=${limit}&offset=${offset}`,
        method: "GET",
      }),
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
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
