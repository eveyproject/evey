// i18n key for a media item's type, the way library.tsx already maps library
// types. Stripping the enum prefix and lowercasing it showed raw English
// ("2024 · movie") to every viewer, in every language (seen on device,
// 2026-08-03).
const KEYS: Record<string, string> = {
  MEDIA_ITEM_TYPE_MOVIE: "media.itemType.movie",
  MEDIA_ITEM_TYPE_SERIES: "media.itemType.series",
  MEDIA_ITEM_TYPE_SEASON: "media.itemType.season",
  MEDIA_ITEM_TYPE_EPISODE: "media.itemType.episode",
  MEDIA_ITEM_TYPE_ALBUM: "media.itemType.album",
  MEDIA_ITEM_TYPE_TRACK: "media.itemType.track",
  MEDIA_ITEM_TYPE_ARTIST: "media.itemType.artist",
  MEDIA_ITEM_TYPE_PHOTO: "media.itemType.photo",
};

// Returns null for an unknown or missing type, so callers show nothing rather
// than a key or a raw enum.
export function mediaTypeLabelKey(type?: string): string | null {
  if (!type) return null;
  return KEYS[type] ?? null;
}
