export function typeLabel(t?: string): string {
  if (t === "MEDIA_ITEM_TYPE_MOVIE") return "Film";
  if (t === "MEDIA_ITEM_TYPE_SERIES") return "Serie";
  return "";
}
