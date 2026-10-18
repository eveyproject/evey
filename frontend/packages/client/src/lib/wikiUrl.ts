// The public wiki, in the viewer's language when it is published there
// (wiki/mkdocs.yml lists them; English is the root, without a prefix).
const WIKI_ORIGIN = "https://eveyproject.org";
const WIKI_LOCALES = new Set(["fr", "de", "es", "hi", "zh"]);

/** A wiki page (path like "privacy/") in the viewer's language. */
export function wikiUrl(lang: string | undefined, path: string): string {
  const code = (lang || "en").split("-")[0].toLowerCase();
  const prefix = WIKI_LOCALES.has(code) ? `/${code}` : "";
  return `${WIKI_ORIGIN}${prefix}/${path.replace(/^\/+/, "")}`;
}
