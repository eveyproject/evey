import { eveyFetch } from "@evey/api";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";

export interface PlaylistRule {
  field: string;
  operator: string;
  value: string;
}

export interface Playlist {
  id: string;
  name: string;
  description: string;
  type: "manual" | "smart";
  rules?: PlaylistRule[];
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

export function usePlaylistList() {
  return useQuery({
    queryKey: ["playlists"],
    queryFn: async () => {
      const r = await eveyFetch<{ playlists: Playlist[] }>({ url: "/api/v1/playlists", method: "GET" });
      return r.playlists ?? [];
    },
    staleTime: 5 * 60_000,
  });
}

interface PlaylistDetailResponse {
  playlist: {
    id: string; name: string; description: string; type: string;
    rules?: PlaylistRule[]; sortBy: string; sortOrder: string;
    createdAt: string; updatedAt: string;
  };
  items?: { mediaItemId: string; sortOrder: number; addedAt: string }[];
  media?: { id: string; title: string; type: string; year?: number }[];
}

export function usePlaylistDetail(id: string) {
  return useQuery({
    queryKey: ["playlists", id],
    queryFn: async (): Promise<Playlist & { items?: PlaylistItem[] }> => {
      const raw = await eveyFetch<PlaylistDetailResponse>({ url: `/api/v1/playlists/${id}`, method: "GET" });
      const pl = raw.playlist;
      const mediaMap = new Map((raw.media ?? []).map((m) => [m.id, m]));
      // For smart playlists, results come in `media` with no corresponding `items`
      const rawItems = raw.items ?? [];
      const items: PlaylistItem[] = rawItems.length > 0
        ? rawItems.map((it) => {
            const m = mediaMap.get(it.mediaItemId);
            return {
              media_item_id: it.mediaItemId,
              sort_order: it.sortOrder,
              added_at: it.addedAt,
              title: m?.title,
              type: m?.type,
              year: m?.year,
            };
          })
        : (raw.media ?? []).map((m, i) => ({
            media_item_id: m.id,
            sort_order: i,
            title: m.title,
            type: m.type,
            year: m.year,
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

export function useCreatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; description?: string; type: "manual" | "smart"; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      eveyFetch<Playlist>({ url: "/api/v1/playlists", method: "POST", data }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["playlists"] }),
  });
}

export function useUpdatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string; name?: string; description?: string; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      eveyFetch<void>({ url: `/api/v1/playlists/${id}`, method: "PUT", data }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["playlists"] }),
  });
}

export function useDeletePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => eveyFetch<void>({ url: `/api/v1/playlists/${id}`, method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["playlists"] }),
  });
}

export function useAddToPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      eveyFetch<void>({ url: `/api/v1/playlists/${playlistId}/items`, method: "POST", data: { mediaItemId } }),
    onSuccess: (_, vars) => qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] }),
  });
}

export function useRemoveFromPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      eveyFetch<void>({ url: `/api/v1/playlists/${playlistId}/items/${mediaItemId}`, method: "DELETE" }),
    onSuccess: (_, vars) => qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] }),
  });
}
