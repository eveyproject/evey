// Shared implementation lives in @evey/client (packages/client/src/api/playlists.ts).
// This module keeps the path the screens and their tests import.
export { usePlaylistList, usePlaylistDetail, useAddToPlaylist, useCreatePlaylist, useGeneratePlaylist, useDeletePlaylist, useUpdatePlaylist, useRemoveFromPlaylist, useReorderPlaylistItems } from "@evey/client";
export type { Playlist, PlaylistItem } from "@evey/client";
