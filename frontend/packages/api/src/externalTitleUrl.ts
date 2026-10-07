// Where a requested title lives on the provider the requester picked it from.
//
// A request names a provider and its id; the operator judging it, and the
// requester reading back their list, both want the provider's own page - the
// one place with the full synopsis, cast and trailer. TMDB honours a language
// parameter on its site, so a French viewer lands on the French page; IMDb and
// MusicBrainz pick the language from the browser, so they get the bare URL.

// TMDB's site wants a locale, not a bare language code: "fr" renders in
// English, "fr-FR" in French. The region is the one the app's six languages
// most plausibly mean; a language outside the list falls back to its own
// upper-cased code (fr -> fr-FR, which is right for the common cases).
const TMDB_LOCALES: Record<string, string> = {
  en: "en-US",
  fr: "fr-FR",
  de: "de-DE",
  es: "es-ES",
  hi: "hi-IN",
  zh: "zh-CN",
};

export function tmdbLocale(language: string | undefined): string {
  const lang = (language ?? "en").toLowerCase();
  if (TMDB_LOCALES[lang]) return TMDB_LOCALES[lang];
  // Already a locale ("pt-BR", "fr_CA"): keep it, normalising the case TMDB
  // expects. A bare code: double it.
  const parts = lang.split(/[-_]/);
  if (parts.length > 1 && parts[1]) return `${parts[0]}-${parts[1].toUpperCase()}`;
  return TMDB_LOCALES[parts[0]] ?? `${parts[0]}-${parts[0].toUpperCase()}`;
}

export interface ExternalTitleRef {
  provider?: string;
  providerId?: string;
  mediaType?: string;
}

/** The provider's display name, for an "Open on {{site}}" label. */
export function externalTitleSite(provider: string | undefined): string | undefined {
  switch (provider) {
    case "tmdb":
      return "TMDB";
    case "imdb":
      return "IMDb";
    case "musicbrainz":
      return "MusicBrainz";
    default:
      return undefined;
  }
}

/**
 * The provider page for a title, or undefined when the request carries no
 * provider id (a free-text request) or names a provider without a public site.
 * The id is URL-encoded: it comes from a provider, but it is still data.
 */
export function externalTitleUrl(ref: ExternalTitleRef, language: string | undefined): string | undefined {
  const id = ref.providerId?.trim();
  if (!id) return undefined;
  const enc = encodeURIComponent(id);
  switch (ref.provider) {
    case "tmdb": {
      const kind = ref.mediaType === "tv" ? "tv" : "movie";
      return `https://www.themoviedb.org/${kind}/${enc}?language=${encodeURIComponent(tmdbLocale(language))}`;
    }
    case "imdb":
      return `https://www.imdb.com/title/${enc}/`;
    case "musicbrainz":
      // The MusicBrainz provider searches release-groups and uses their id.
      return `https://musicbrainz.org/release-group/${enc}`;
    default:
      return undefined;
  }
}
