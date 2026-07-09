// Route builders for the music drill-down (artist -> albums -> tracks). The
// album track listing is scoped by libraryId server-side (ListTracksByAlbumID
// resolves the opaque albumId against that library's folders only), so every
// link into an album or artist screen must carry the source libraryId along.
export function musicArtistHref(artistId: string, libraryId: string): string {
  return `/(auth)/music/artist/${encodeURIComponent(artistId)}?libraryId=${encodeURIComponent(libraryId)}`;
}

export function musicAlbumHref(albumId: string, libraryId: string): string {
  return `/(auth)/music/album/${encodeURIComponent(albumId)}?libraryId=${encodeURIComponent(libraryId)}`;
}
