// Safe projection of a library item, as returned by the host's `public`
// permission surface (media.list / media.get). Never carries file paths,
// sizes, provider ids or genres/ratings - the host does not expose those
// through `public` (see proto PublicMediaItem: id/type/title/year/overview/
// poster_url only). The galaxy is therefore built from year (-> decade) and
// type (movie vs series), the only two dimensions actually available.

export type MediaKind = "movie" | "series" | "other";

export interface CinephileItem {
  id: string;
  title: string;
  year?: number;
  posterUrl?: string;
  type: MediaKind;
}
