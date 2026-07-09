// Safe projection of a library item, as returned by the host's `public`
// permission surface (media.list / media.get). Never carries file paths,
// sizes or provider ids - only what a poster wall needs to draw and link to.

export interface MediaSummary {
  id: string;
  title: string;
  year?: number;
  posterUrl: string;
}
