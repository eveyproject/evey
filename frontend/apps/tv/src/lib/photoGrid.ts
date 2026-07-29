// Pure helpers for the TV photo grid (app/(auth)/photos/index.tsx). Kept
// separate from the screen so the URL/param shaping can be unit-tested
// without rendering FlatList/Image.

// Local mirror of the generated photo.schemas response shapes. The
// orval-generated photo-service.ts does not re-export V1Photo/V1PhotoAlbum
// through the @evey/api package barrel (only a hand-picked subset of schema
// files is), and that barrel is out of scope for this change. Field names
// are lowerCamelCase because /api/v1/photos* is served by the grpc-gateway
// (protojson) - mirrors the mobile app's identical local copy of this shape.
export interface TvPhoto {
  id?: string;
  title?: string;
  album?: string;
  blurhash?: string;
}

export interface TvPhotoAlbum {
  name?: string;
  libraryId?: string;
  photoCount?: number;
  coverPhotoId?: string;
}

// Mirrors internal/adapters/driving/http/photo_handler.go's photoAllowedWidths.
// The server snaps any ?w= value to the nearest entry in this list, but the
// grid must still request one of them on purpose: it caps how many distinct
// on-disk variants a single photo can generate, and it keeps a weak NAS/box
// from ever being asked to hand back a full-resolution photo (several MB) for
// a thumbnail tile.
export const SERVER_ALLOWED_PHOTO_WIDTHS = [200, 400, 800, 1600] as const;

// The grid never needs more than a 400px-wide tile even on a dense TV
// display (tiles are rendered well under that), and 400 is already one of
// the server's allowed widths - no snapping, no wasted variant.
export const PHOTO_GRID_THUMB_WIDTH = 400;

export interface PhotoServeUrlOptions {
  width?: number;
  format?: string;
}

// Builds the authenticated-fetch URL for GET /api/v1/photos/{id}/serve. No
// width omits it entirely, which serves the original file - used by the
// fullscreen slideshow, never by the grid.
export function photoServeUrl(serverUrl: string, photoId: string, opts?: PhotoServeUrlOptions): string {
  const base = `${serverUrl}/api/v1/photos/${photoId}/serve`;
  const params = new URLSearchParams();
  if (opts?.width) params.set("w", String(opts.width));
  if (opts?.format) params.set("fmt", opts.format);
  const qs = params.toString();
  return qs ? `${base}?${qs}` : base;
}

export interface BuildPhotoListParamsInput {
  libraryId: string;
  album?: string;
  limit: number;
  offset: number;
}

// Single source of truth for the ListPhotos query params (offset-paginated -
// the photo API has no opaque cursor like ListMediaItems).
export function buildPhotoListParams(input: BuildPhotoListParamsInput) {
  return {
    libraryId: input.libraryId,
    limit: input.limit,
    offset: input.offset,
    ...(input.album ? { album: input.album } : {}),
  };
}

// Whether another page is worth fetching. total<=0 covers both "no photos"
// and a response that omitted total (older server) - either way, stop.
export function hasMorePhotos(loadedCount: number, total: number): boolean {
  return total > 0 && loadedCount < total;
}
