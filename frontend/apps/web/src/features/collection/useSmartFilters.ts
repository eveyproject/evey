import { usePlaylistList, useDeletePlaylist } from "@/features/playlist/api";
import { playlistToSmartFilter, type SmartFilter } from "./SmartCollectionDialog";

// Smart filters are backed by smart playlists on the server.
// This hook provides a compatible interface used by the collections page.
export function useSmartFilters() {
  const { data: playlists, isLoading } = usePlaylistList();
  const deleteMut = useDeletePlaylist();

  const filters: SmartFilter[] = (playlists ?? [])
    .filter((pl) => pl.type === "smart")
    .map(playlistToSmartFilter);

  function save(_filter: SmartFilter) {
    // create/update is handled inside SmartCollectionDialog via mutations;
    // this callback exists for interface compatibility but is a no-op here.
  }

  function remove(id: string) {
    deleteMut.mutate(id);
  }

  return { filters, save, remove, isLoading };
}
