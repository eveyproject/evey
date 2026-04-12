import { useMutation, useQueryClient } from "@tanstack/react-query";
import {
  usePlaylistServiceListPlaylists,
  usePlaylistServiceGetPlaylist,
  usePlaylistServiceGetPlaylistItems,
  playlistServiceCreatePlaylist,
  playlistServiceUpdatePlaylist,
  playlistServiceDeletePlaylist,
  playlistServiceAddPlaylistItem,
  playlistServiceRemovePlaylistItem,
  getPlaylistServiceListPlaylistsQueryKey,
  getPlaylistServiceGetPlaylistQueryKey,
  getPlaylistServiceGetPlaylistItemsQueryKey,
  V1PlaylistType,
} from "@evey/api";
import type { V1PlaylistProto } from "@evey/api";

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

function typeToLocal(t?: V1PlaylistType): "manual" | "smart" {
  return t === V1PlaylistType.PLAYLIST_TYPE_SMART ? "smart" : "manual";
}

function typeToProto(t: "manual" | "smart"): V1PlaylistType {
  return t === "smart" ? V1PlaylistType.PLAYLIST_TYPE_SMART : V1PlaylistType.PLAYLIST_TYPE_MANUAL;
}

function mapPlaylist(p: V1PlaylistProto): Playlist {
  return {
    id: p.id ?? "",
    name: p.name ?? "",
    description: p.description ?? "",
    type: typeToLocal(p.type),
    rules: p.rules?.map((r) => ({ field: r.field ?? "", operator: r.operator ?? "", value: r.value ?? "" })),
    sort_by: p.sortBy ?? "",
    sort_order: p.sortOrder ?? "",
    created_at: p.createdAt ?? "",
    updated_at: p.updatedAt ?? "",
  };
}

export function usePlaylistList() {
  return usePlaylistServiceListPlaylists(undefined, {
    query: {
      select: (d) => (d.playlists ?? []).map(mapPlaylist),
      staleTime: 5 * 60_000,
    },
  });
}

export function usePlaylistDetail(id: string) {
  const head = usePlaylistServiceGetPlaylist(id, undefined, {
    query: { enabled: !!id, staleTime: 5 * 60_000, select: mapPlaylist },
  });
  const items = usePlaylistServiceGetPlaylistItems(id, undefined, {
    query: {
      enabled: !!id,
      staleTime: 5 * 60_000,
      select: (d) =>
        (d.items ?? []).map<PlaylistItem>((it) => ({
          media_item_id: it.mediaItemId ?? "",
          sort_order: it.sortOrder,
          added_at: it.addedAt,
          title: it.title,
          type: it.type,
          year: it.year,
        })),
    },
  });
  return {
    data: head.data ? { ...head.data, items: items.data ?? [] } : undefined,
    isLoading: head.isLoading || items.isLoading,
    isError: head.isError || items.isError,
    error: head.error ?? items.error,
  };
}

export function useCreatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (data: { name: string; description?: string; type: "manual" | "smart"; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      playlistServiceCreatePlaylist({
        name: data.name,
        description: data.description,
        type: typeToProto(data.type),
        rules: data.rules,
        sortBy: data.sort_by,
        sortOrder: data.sort_order,
      }),
    onSuccess: () => qc.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() }),
  });
}

export function useUpdatePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ id, ...data }: { id: string; name?: string; description?: string; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      playlistServiceUpdatePlaylist(id, {
        name: data.name,
        description: data.description,
        rules: data.rules,
        sortBy: data.sort_by,
        sortOrder: data.sort_order,
      }),
    onSuccess: (_d, vars) => {
      qc.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() });
      qc.invalidateQueries({ queryKey: getPlaylistServiceGetPlaylistQueryKey(vars.id) });
    },
  });
}

export function useDeletePlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => playlistServiceDeletePlaylist(id),
    onSuccess: () => qc.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() }),
  });
}

export function useAddToPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      playlistServiceAddPlaylistItem(playlistId, { mediaItemId }),
    onSuccess: (_, vars) => qc.invalidateQueries({ queryKey: getPlaylistServiceGetPlaylistItemsQueryKey(vars.playlistId) }),
  });
}

export function useRemoveFromPlaylist() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      playlistServiceRemovePlaylistItem(playlistId, mediaItemId),
    onSuccess: (_, vars) => qc.invalidateQueries({ queryKey: getPlaylistServiceGetPlaylistItemsQueryKey(vars.playlistId) }),
  });
}
