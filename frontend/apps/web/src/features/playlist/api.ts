// Shared implementation lives in @evey/client (packages/client/src/api/playlists.ts),
// the same layer the phone and the televisions use. This module keeps the path
// the web's screens and tests import.
export {
  usePlaylistList,
  usePlaylistDetail,
  useCreatePlaylist,
  useUpdatePlaylist,
  useGeneratePlaylist,
  useDeletePlaylist,
  useAddToPlaylist,
  useRemoveFromPlaylist,
  useReorderPlaylistItems,
} from "@evey/client";
export type { Playlist, PlaylistItem, PlaylistRule } from "@evey/client";
