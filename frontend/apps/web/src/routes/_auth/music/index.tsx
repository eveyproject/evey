import { createFileRoute } from "@tanstack/react-router";

type MusicSearch = {
  tab?: "albums" | "tracks" | "artists" | "playlists";
  album?: string;
  artist?: string;
  libraryId?: string;
  playlistId?: string;
};

export const Route = createFileRoute("/_auth/music/")({
  validateSearch: (search: Record<string, unknown>): MusicSearch => {
    const tab = typeof search.tab === "string" ? search.tab : undefined;
    return {
      tab: tab === "albums" || tab === "tracks" || tab === "artists" || tab === "playlists" ? tab : undefined,
      album: typeof search.album === "string" ? search.album : undefined,
      artist: typeof search.artist === "string" ? search.artist : undefined,
      libraryId: typeof search.libraryId === "string" ? search.libraryId : undefined,
      playlistId: typeof search.playlistId === "string" ? search.playlistId : undefined,
    };
  },
});
