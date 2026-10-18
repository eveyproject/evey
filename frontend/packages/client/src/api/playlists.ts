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
  eveyFetch,
} from "@evey/api";
import type { V1PlaylistProto } from "@evey/api";

// The playlists of every client. The web kept its own layer beside this one
// until 2026-09-24, and the copies had drifted: this one passed the API's enum
// type straight to the screens (every add-to-playlist sheet of the phone and
// both televisions showed nothing) while the web's translated it.
//
// These hooks say nothing to the viewer: the screen that calls them words its
// own message (toast), which is how the web names the playlist it added to.

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

// The API speaks its enum ("PLAYLIST_TYPE_MANUAL"); the screens speak
// "manual" | "smart". Creating with type "manual" is refused (HTTP 400).
function typeToLocal(t?: string): "manual" | "smart" {
  return t === V1PlaylistType.PLAYLIST_TYPE_SMART || t === "smart" ? "smart" : "manual";
}

function typeToProto(t: string): V1PlaylistType {
  return typeToLocal(t) === "smart" ? V1PlaylistType.PLAYLIST_TYPE_SMART : V1PlaylistType.PLAYLIST_TYPE_MANUAL;
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
  // The members come from their own endpoint, already carrying title, type
  // and year.
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
    // Both halves have to come back for the page to render, so a retry has to
    // re-run both - retrying only the one that failed leaves the other stale.
    refetch: async () => {
      await Promise.all([head.refetch(), items.refetch()]);
    },
  };
}

function useInvalidate() {
  const qc = useQueryClient();
  return {
    list: () => qc.invalidateQueries({ queryKey: getPlaylistServiceListPlaylistsQueryKey() }),
    head: (id: string) => qc.invalidateQueries({ queryKey: getPlaylistServiceGetPlaylistQueryKey(id) }),
    items: (id: string) => qc.invalidateQueries({ queryKey: getPlaylistServiceGetPlaylistItemsQueryKey(id) }),
  };
}

export function useCreatePlaylist() {
  const invalidate = useInvalidate();
  return useMutation({
    // The API answers with the playlist itself, flat: the phone read
    // res.playlist.id and never found the new one.
    mutationFn: async (data: {
      name: string;
      description?: string;
      type: string;
      rules?: PlaylistRule[];
      sort_by?: string;
      sort_order?: string;
    }): Promise<{ id: string }> => {
      const created = await playlistServiceCreatePlaylist({
        name: data.name,
        description: data.description,
        type: typeToProto(data.type),
        rules: data.rules,
        sortBy: data.sort_by,
        sortOrder: data.sort_order,
      });
      return { id: created.id ?? "" };
    },
    onSuccess: () => invalidate.list(),
  });
}

// ownErrorToast: the caller toasts its own failure (a specific message), so the
// global mutation error net must stay quiet or the viewer gets two toasts.
export function useUpdatePlaylist(opts: { ownErrorToast?: boolean } = {}) {
  const invalidate = useInvalidate();
  return useMutation({
    meta: opts.ownErrorToast ? { silentError: true } : undefined,
    mutationFn: ({ id, ...data }: { id: string; name?: string; description?: string; rules?: PlaylistRule[]; sort_by?: string; sort_order?: string }) =>
      playlistServiceUpdatePlaylist(id, {
        name: data.name,
        description: data.description,
        rules: data.rules,
        sortBy: data.sort_by,
        sortOrder: data.sort_order,
      }),
    onSuccess: (_d, vars) => {
      void invalidate.list();
      void invalidate.head(vars.id);
      // A smart playlist's rules decide which items match, so editing them
      // changes the items list - refresh it too, not just the metadata.
      void invalidate.items(vars.id);
    },
  });
}

// Builds a taste-weighted music playlist on the server (custom endpoint, not
// part of the gRPC gateway) and returns its id.
export function useGeneratePlaylist() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (data: { libraryId: string; name?: string; size?: number }) =>
      eveyFetch<{ id: string; name: string }>({ url: "/api/v1/playlists/generate", method: "POST", data }),
    onSuccess: () => invalidate.list(),
  });
}

export function useDeletePlaylist() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: (id: string) => playlistServiceDeletePlaylist(id),
    onSuccess: () => invalidate.list(),
  });
}

export function useAddToPlaylist() {
  const invalidate = useInvalidate();
  return useMutation({
    // mediaType tells a music track from a video; absent, the server files it
    // as a video.
    mutationFn: ({ playlistId, mediaItemId, mediaType }: { playlistId: string; mediaItemId: string; mediaType?: string }) =>
      playlistServiceAddPlaylistItem(playlistId, mediaType ? { mediaItemId, mediaType } : { mediaItemId }),
    onSuccess: (_, vars) => invalidate.items(vars.playlistId),
  });
}

export function useRemoveFromPlaylist() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ playlistId, mediaItemId }: { playlistId: string; mediaItemId: string }) =>
      playlistServiceRemovePlaylistItem(playlistId, mediaItemId),
    onSuccess: (_, vars) => invalidate.items(vars.playlistId),
  });
}

// The hand-set order of a playlist's members (custom endpoint).
export function useReorderPlaylistItems() {
  const invalidate = useInvalidate();
  return useMutation({
    mutationFn: ({ playlistId, items }: { playlistId: string; items: { mediaItemId: string; sortOrder: number }[] }) =>
      eveyFetch<void>({ url: `/api/v1/playlists/${playlistId}/items/reorder`, method: "PUT", data: { items } }),
    onSuccess: (_, vars) => invalidate.items(vars.playlistId),
  });
}
