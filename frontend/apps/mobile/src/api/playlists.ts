import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import i18n from "../i18n";

export interface Playlist {
  id: string;
  name: string;
  description: string;
  type: "manual" | "smart";
  rules?: { field: string; operator: string; value: string }[];
  sort_by: string;
  sort_order: string;
  created_at: string;
  updated_at: string;
}

export interface PlaylistItem {
  media_item_id: string;
  sort_order?: number;
  added_at?: string;
  title?: string;
  type?: string;
  year?: number;
}

// GET /api/v1/playlists/{id} answers with the playlist itself, flat - there is
// no wrapper, no items and no media alongside it. Claiming otherwise made
// raw.playlist undefined and opening a playlist threw.
interface PlaylistDetailResponse {
  id: string;
  name: string;
  description: string;
  type: string;
  rules?: { field: string; operator: string; value: string }[];
  sortBy: string;
  sortOrder: string;
  createdAt: string;
  updatedAt: string;
}

// The members come from their own endpoint, already carrying the title, type
// and year - the web reads them the same way.
interface PlaylistItemsResponse {
  items?: { mediaItemId?: string; sortOrder?: number; addedAt?: string; title?: string; type?: string; year?: number }[];
}

export function usePlaylistList() {
  return useQuery({
    queryKey: ["playlists"],
    queryFn: async () => {
      const r = await eveyFetch<{ playlists: Playlist[] }>({
        url: "/api/v1/playlists",
        method: "GET",
      });
      return r.playlists ?? [];
    },
    staleTime: 5 * 60_000,
  });
}

export function usePlaylistDetail(id: string) {
  return useQuery({
    queryKey: ["playlists", id],
    queryFn: async (): Promise<Playlist & { items?: PlaylistItem[] }> => {
      const [pl, itemsRes] = await Promise.all([
        eveyFetch<PlaylistDetailResponse>({ url: `/api/v1/playlists/${id}`, method: "GET" }),
        eveyFetch<PlaylistItemsResponse>({ url: `/api/v1/playlists/${id}/items`, method: "GET" }),
      ]);
      const items: PlaylistItem[] = (itemsRes.items ?? []).map((it) => ({
        media_item_id: it.mediaItemId ?? "",
        sort_order: it.sortOrder,
        added_at: it.addedAt,
        title: it.title,
        type: it.type,
        year: it.year,
      }));
      return {
        id: pl.id,
        name: pl.name,
        description: pl.description,
        type: pl.type as "manual" | "smart",
        rules: pl.rules,
        sort_by: pl.sortBy,
        sort_order: pl.sortOrder,
        created_at: pl.createdAt,
        updated_at: pl.updatedAt,
        items,
      };
    },
    enabled: !!id,
    staleTime: 5 * 60_000,
  });
}

export function useAddToPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({
      playlistId,
      mediaItemId,
      mediaType,
    }: {
      playlistId: string;
      mediaItemId: string;
      mediaType?: string;
    }) =>
      eveyFetch<void>({
        url: `/api/v1/playlists/${playlistId}/items`,
        method: "POST",
        data: mediaType ? { mediaItemId, mediaType } : { mediaItemId },
      }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] });
      toast.success(i18n.t("media.playlist.addSuccess"));
    },
    onError: () => toast.error(i18n.t("media.playlist.addError")),
  });
}

export function useCreatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; description?: string; type: string }) =>
      eveyFetch<{ playlist: { id: string } }>({
        url: "/api/v1/playlists",
        method: "POST",
        data,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["playlists"] });
      toast.success(i18n.t("media.playlist.createSuccess"));
    },
    onError: () => toast.error(i18n.t("media.playlist.createError")),
  });
}

export function useDeletePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) =>
      eveyFetch<void>({
        url: `/api/v1/playlists/${id}`,
        method: "DELETE",
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["playlists"] });
      toast.success(i18n.t("media.playlist.playlistDeleted"));
    },
    onError: () => toast.error(i18n.t("media.playlist.deleteFailed")),
  });
}

export function useUpdatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...body }: { id: string; name?: string; description?: string; rules?: { field: string; operator: string; value: string }[] }) =>
      eveyFetch<void>({
        url: `/api/v1/playlists/${id}`,
        // The proto declares UpdatePlaylist as PATCH (proto/evey/v1/playlist.proto);
        // PUT 404'd against grpc-gateway's generated mux.
        method: "PATCH",
        data: body,
      }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["playlists", vars.id] });
      qc.invalidateQueries({ queryKey: ["playlists"] });
      toast.success(i18n.t("media.playlist.updated"));
    },
    onError: () => toast.error(i18n.t("media.playlist.updateFailed")),
  });
}

export function useRemoveFromPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      eveyFetch<void>({
        url: `/api/v1/playlists/${playlistId}/items/${mediaItemId}`,
        method: "DELETE",
      }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] });
      toast.success(i18n.t("media.playlist.removedFromPlaylist"));
    },
    onError: () => toast.error(i18n.t("media.playlist.failedRemove")),
  });
}

export function useReorderPlaylistItems() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, items }: { playlistId: string; items: { mediaItemId: string; sortOrder: number }[] }) =>
      eveyFetch<void>({
        url: `/api/v1/playlists/${playlistId}/items/reorder`,
        method: "PUT",
        data: { items },
      }),
    onSuccess: (_, vars) => {
      qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] });
    },
    onError: () => toast.error(i18n.t("media.playlist.reorderFailed")),
  });
}
