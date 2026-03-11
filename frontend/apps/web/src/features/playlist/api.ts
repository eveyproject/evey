import { useAuthStore } from "@evey/store";
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

function authHeaders(): Record<string, string> {
  const token = useAuthStore.getState().accessToken;
  return token ? { Authorization: `Bearer ${token}`, "Content-Type": "application/json" } : { "Content-Type": "application/json" };
}

async function apiFetch<T>(url: string, opts?: RequestInit): Promise<T> {
  const res = await fetch(url, { ...opts, headers: { ...authHeaders(), ...(opts?.headers ?? {}) } });
  if (!res.ok) {
    const body = await res.json().catch(() => ({})) as { message?: string };
    throw new Error(body.message ?? `${res.status}`);
  }
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export function usePlaylistList() {
  return useQuery({
    queryKey: ["playlists"],
    queryFn: () => apiFetch<{ playlists: Playlist[] }>("/api/v1/playlists").then((r) => r.playlists ?? []),
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
      const raw = await apiFetch<PlaylistDetailResponse>(`/api/v1/playlists/${id}`);
      const pl = raw.playlist;
      const mediaMap = new Map((raw.media ?? []).map((m) => [m.id, m]));
      const items: PlaylistItem[] = (raw.items ?? []).map((it) => {
        const m = mediaMap.get(it.mediaItemId);
        return {
          media_item_id: it.mediaItemId,
          sort_order: it.sortOrder,
          added_at: it.addedAt,
          title: m?.title,
          type: m?.type,
          year: m?.year,
        };
      });
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
  });
}

export function useCreatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; description?: string; type: "manual" | "smart"; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      apiFetch<Playlist>("/api/v1/playlists", { method: "POST", body: JSON.stringify(data) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["playlists"] }),
  });
}

export function useUpdatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string; name?: string; description?: string; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      apiFetch<void>(`/api/v1/playlists/${id}`, { method: "PUT", body: JSON.stringify(data) }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["playlists"] }),
  });
}

export function useDeletePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => apiFetch<void>(`/api/v1/playlists/${id}`, { method: "DELETE" }),
    onSuccess: () => qc.invalidateQueries({ queryKey: ["playlists"] }),
  });
}

export function useAddToPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      apiFetch<void>(`/api/v1/playlists/${playlistId}/items`, { method: "POST", body: JSON.stringify({ mediaItemId }) }),
    onSuccess: (_, vars) => qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] }),
  });
}

export function useRemoveFromPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      apiFetch<void>(`/api/v1/playlists/${playlistId}/items/${mediaItemId}`, { method: "DELETE" }),
    onSuccess: (_, vars) => qc.invalidateQueries({ queryKey: ["playlists", vars.playlistId] }),
  });
}
