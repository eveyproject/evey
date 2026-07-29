// Route builder for the TV photo slideshow. startIndex travels as a query
// param (not encoded into the loaded list itself) so the slideshow screen can
// independently fetch the same ordered ListPhotos page and land on the photo
// the user actually pressed in the grid.
export function photoSlideshowHref(libraryId: string, startIndex: number, album?: string): string {
  const params = new URLSearchParams();
  params.set("libraryId", libraryId);
  params.set("startIndex", String(startIndex));
  if (album) params.set("album", album);
  return `/(auth)/photos/slideshow?${params.toString()}`;
}
