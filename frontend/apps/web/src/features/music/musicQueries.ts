import {
  useLibraryServiceListLibraries,
  useMusicServiceListTracks,
  useMusicServiceGetAlbums,
  useMusicServiceGetArtists,
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

export function useMusicTracks(libraryID: string | undefined, search: string, albumId?: string) {
  return useMusicServiceListTracks(
    // When albumId is set the server returns that album's full track set and
    // ignores search/limit; the in-tab search box then filters it client-side.
    { libraryId: libraryID, limit: 200, search: albumId ? undefined : (search || undefined), albumId: albumId || undefined },
    {
      query: {
        enabled: !!libraryID,
        select: (d) => (d.tracks ?? []).map<Track>((t) => ({
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
        })),
        staleTime: 60_000,
      },
    },
  );
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
