import { describe, it, expect } from "vitest";
import { externalTitleSite, externalTitleUrl, tmdbLocale } from "./externalTitleUrl";

describe("externalTitleUrl", () => {
  it("sends a TMDB movie to its page in the viewer's language", () => {
    expect(externalTitleUrl({ provider: "tmdb", providerId: "550", mediaType: "movie" }, "fr")).toBe(
      "https://www.themoviedb.org/movie/550?language=fr-FR",
    );
  });

  it("uses the tv path for a series", () => {
    expect(externalTitleUrl({ provider: "tmdb", providerId: "1399", mediaType: "tv" }, "de")).toBe(
      "https://www.themoviedb.org/tv/1399?language=de-DE",
    );
  });

  it("links IMDb and MusicBrainz without a language, which their sites ignore", () => {
    expect(externalTitleUrl({ provider: "imdb", providerId: "tt0137523", mediaType: "movie" }, "fr")).toBe(
      "https://www.imdb.com/title/tt0137523/",
    );
    expect(externalTitleUrl({ provider: "musicbrainz", providerId: "abc-123", mediaType: "music" }, "fr")).toBe(
      "https://musicbrainz.org/release-group/abc-123",
    );
  });

  it("has no link for a free-text request or an unknown provider", () => {
    expect(externalTitleUrl({ provider: "tmdb", providerId: "", mediaType: "movie" }, "en")).toBeUndefined();
    expect(externalTitleUrl({ provider: "omdb", providerId: "x", mediaType: "movie" }, "en")).toBeUndefined();
  });

  it("encodes the id - it is provider data, not a trusted path", () => {
    expect(externalTitleUrl({ provider: "imdb", providerId: "tt1/../x", mediaType: "movie" }, "en")).toBe(
      "https://www.imdb.com/title/tt1%2F..%2Fx/",
    );
  });

  it("names the site for the label", () => {
    expect(externalTitleSite("tmdb")).toBe("TMDB");
    expect(externalTitleSite("imdb")).toBe("IMDb");
    expect(externalTitleSite("musicbrainz")).toBe("MusicBrainz");
    expect(externalTitleSite("")).toBeUndefined();
  });
});

describe("tmdbLocale", () => {
  it("maps the app's languages to the locale TMDB's site needs", () => {
    expect(tmdbLocale("fr")).toBe("fr-FR");
    expect(tmdbLocale("zh")).toBe("zh-CN");
    expect(tmdbLocale("hi")).toBe("hi-IN");
    expect(tmdbLocale(undefined)).toBe("en-US");
  });

  it("keeps a locale it is handed and doubles a bare code it does not know", () => {
    expect(tmdbLocale("pt-BR")).toBe("pt-BR");
    expect(tmdbLocale("it")).toBe("it-IT");
    expect(tmdbLocale("fr_CA")).toBe("fr-CA");
  });
});
