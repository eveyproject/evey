import { useMemo } from "react";
import { useInfiniteQuery } from "@tanstack/react-query";
import {
  useLibraryServiceListLibraries,
  musicServiceListTracks,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
  hasMoreFromCursor,
} from "@evey/api";

export type Track = {
  ID: string;
  Title: string;
  Artist: string;
  Album: string;
  AlbumArtist: string;
  Year: number;
  TrackNumber: number;
  DiskNumber: number;
  Duration: number;
  FilePath: string;
  MimeType: string;
  Genre: string;
  CoverArtURL: string;
};

export type Album = {
  ID: string;
  Title: string;
  Artist: string;
  Year: number;
  CoverArtURL: string;
  TrackCount: number;
  Duration: number;
};

export type Artist = {
  ID: string;
  Name: string;
  TrackCount: number;
  AlbumCount: number;
  CoverArtURL: string;
};

interface LibraryItem {
  id: string;
  name: string;
  type: string;
}

export function useMusicLibraries() {
  return useLibraryServiceListLibraries({
    query: {
      select: (d) => (d.libraries ?? []).filter((l) => l.type === "LIBRARY_TYPE_MUSIC").map<LibraryItem>((l) => ({ id: l.id ?? "", name: l.name ?? "", type: l.type ?? "" })),
      staleTime: 60_000,
    },
  });
}

// Tracks are keyset-paginated. The tab used to ask for a flat limit=200, which
// simply lost everything past the 200th track of a library: the pager then
// reported "1 / 4 - 200 tracks" on a collection of a thousand, and no click
// could reach the rest.
//
// Only the plain list path pages. With an albumId the server returns that
// album's full track set and ignores limit/search entirely, and a free-text
// search is answered by the full-text index, which has no keyset of its own -
// both come back as a single page with no cursor, so hasNextPage is false and
// the tab behaves exactly as before.
const TRACK_PAGE_SIZE = 200;

function toTrack(t: NonNullable<Awaited<ReturnType<typeof musicServiceListTracks>>["tracks"]>[number]): Track {
  return {
    ID: t.id ?? "",
    Title: t.title ?? "",
    Artist: t.artist ?? "",
    Album: t.album ?? "",
    AlbumArtist: t.albumArtist ?? "",
    Year: t.year ?? 0,
    TrackNumber: t.trackNumber ?? 0,
    DiskNumber: t.diskNumber ?? 0,
    Duration: t.duration ?? 0,
    FilePath: t.filePath ?? "",
    MimeType: t.mimeType ?? "",
    Genre: t.genre ?? "",
    CoverArtURL: t.coverArtUrl ?? "",
  };
}

// SHUFFLE_QUEUE_SIZE is how many tracks a "play random" draws from the
// server: a long queue, not the whole library.
export const SHUFFLE_QUEUE_SIZE = 200;

// fetchShuffledTracks draws random tracks from the WHOLE library server-side,
// instead of shuffling only the pages this screen happened to load.
export async function fetchShuffledTracks(libraryID: string, limit = SHUFFLE_QUEUE_SIZE): Promise<Track[]> {
  const res = await musicServiceListTracks({ libraryId: libraryID, shuffle: true, limit });
  return (res.tracks ?? []).map(toTrack);
}

export function useMusicTracks(libraryID: string | undefined, search: string, albumId?: string) {
  const effectiveSearch = albumId ? undefined : (search || undefined);
  const query = useInfiniteQuery({
    queryKey: ["music-tracks-infinite", libraryID ?? "", effectiveSearch ?? "", albumId ?? ""],
    enabled: !!libraryID,
    initialPageParam: "" as string,
    queryFn: ({ pageParam, signal }) =>
      musicServiceListTracks(
        {
          libraryId: libraryID,
          limit: TRACK_PAGE_SIZE,
          ...(effectiveSearch ? { search: effectiveSearch } : {}),
          ...(albumId ? { albumId } : {}),
          ...(pageParam ? { cursor: pageParam } : { offset: 0 }),
        },
        signal,
      ),
    getNextPageParam: (last) => (hasMoreFromCursor(last) ? last.nextCursor : undefined),
    staleTime: 60_000,
  });

  const data = useMemo(
    () =>
      (query.data?.pages ?? []).flatMap((page) =>
        (page.tracks ?? []).map(toTrack),
      ),
    [query.data],
  );

  return { ...query, data };
}

export function useMusicAlbums(libraryID: string | undefined) {
  return useMusicServiceGetAlbums(
    { libraryId: libraryID },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.albums ?? []).map<Album>((a) => ({
          ID: a.id ?? "",
          Title: a.title ?? "",
          Artist: a.artist ?? "",
          Year: a.year ?? 0,
          CoverArtURL: a.coverArtUrl ?? "",
          TrackCount: a.trackCount ?? 0,
          Duration: a.duration ?? 0,
        })),
        staleTime: 60_000,
      },
    },
  );
}

export function useMusicArtists(libraryID: string | undefined) {
  return useMusicServiceGetArtists(
    { libraryId: libraryID },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.artists ?? []).map<Artist>((a) => ({
          ID: a.id ?? "",
          Name: a.name ?? "",
          TrackCount: a.trackCount ?? 0,
          AlbumCount: a.albumCount ?? 0,
          CoverArtURL: a.coverArtUrl ?? "",
        })),
        staleTime: 60_000,
      },
    },
  );
}
