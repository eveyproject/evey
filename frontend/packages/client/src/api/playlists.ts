import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import i18n from "i18next";

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

// The API speaks its enum ("PLAYLIST_TYPE_MANUAL"); the screens speak
// "manual" | "smart". Nothing translated before 2026-09-24: every
// `type === "manual"` filter kept nothing, so the phone's and both
// televisions' add-to-playlist sheets said there was no playlist at all
// (measured in the browser television with two manual playlists on the
// server), and creating one with type "manual" was refused (HTTP 400).
function localType(type?: string): "manual" | "smart" {
  return type === "smart" || type === "PLAYLIST_TYPE_SMART" ? "smart" : "manual";
}

function protoType(type: string): string {
  return localType(type) === "smart" ? "PLAYLIST_TYPE_SMART" : "PLAYLIST_TYPE_MANUAL";
}

interface PlaylistListResponse {
  playlists?: (Partial<PlaylistDetailResponse> & { id: string })[];
}

export function usePlaylistList() {
  return useQuery({
    queryKey: ["playlists"],
    queryFn: async () => {
      const r = await eveyFetch<PlaylistListResponse>({
        url: "/api/v1/playlists",
        method: "GET",
      });
      return (r.playlists ?? []).map<Playlist>((p) => ({
        id: p.id,
        name: p.name ?? "",
        description: p.description ?? "",
        type: localType(p.type),
        rules: p.rules,
        sort_by: p.sortBy ?? "",
        sort_order: p.sortOrder ?? "",
        created_at: p.createdAt ?? "",
        updated_at: p.updatedAt ?? "",
      }));
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
        type: localType(pl.type),
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
    // The API answers with the playlist itself, flat: reading res.playlist.id
    // (as the phone did) always found nothing, so "create and add" never
    // added and the new playlist never opened.
    mutationFn: async (data: { name: string; description?: string; type: string }): Promise<{ id: string }> => {
      const created = await eveyFetch<{ id: string }>({
        url: "/api/v1/playlists",
        method: "POST",
        data: { ...data, type: protoType(data.type) },
      });
      return { id: created.id };
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["playlists"] });
      toast.success(i18n.t("media.playlist.createSuccess"));
    },
    onError: () => toast.error(i18n.t("media.playlist.createError")),
  });
}

// POST /api/v1/playlists/generate builds a taste-weighted music playlist
// server-side and answers with its id - the same custom endpoint the web's
// useGeneratePlaylist calls (it is not part of the gRPC gateway).
export function useGeneratePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { libraryId: string; name?: string; size?: number }) =>
      eveyFetch<{ id: string; name: string }>({
        url: "/api/v1/playlists/generate",
        method: "POST",
        data,
      }),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["playlists"] });
    },
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
