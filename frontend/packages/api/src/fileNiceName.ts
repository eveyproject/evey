/**
 * The title a human would read off a filename.
 *
 * `/media/films/Rocket 2 - 1080P.mkv` -> `Rocket 2`
 *
 * Used to show admins what is actually on disk while they edit a fiche, and to
 * seed the TMDB identify box - which otherwise starts from the stored title,
 * i.e. from whatever the previous (possibly wrong) match wrote there. Starting
 * from the file is the whole point: when a match is wrong, the filename is the
 * one piece of evidence that has not been overwritten.
 *
 * This is a DISPLAY helper, deliberately simpler than the scan-time parser in
 * `internal/adapters/driven/filesystem/parser.go`. That one also has to decide
 * movie vs series, extract season/episode, language, release group and edition
 * suffixes, and its behaviour is load-bearing for matching. Duplicating it in
 * TypeScript would guarantee drift on the part that matters. Here the cost of
 * being slightly off is a suggestion the admin edits - so the rule is: strip
 * what is unambiguously technical, keep everything else.
 */

// Ordered: episode markers first (they anchor where the title ends), then the
// technical tags. Each is matched as a whole token so a title is never gutted -
// "Rocket 2" keeps its 2, and a film called "Alien" is not truncated by "ali".
const TOKEN_PATTERNS: RegExp[] = [
  // Season/episode: S01E05, S01 E05, 1x05
  /\bS\d{1,2}\s?E\d{1,3}\b/i,
  /\b\d{1,2}x\d{2,3}\b/i,
  // Resolution and source
  /\b(?:2160p?|1080p?|1440p?|720p?|576p?|480p?|4k|8k|uhd|hd|sd)\b/i,
  /\b(?:bluray|blu-ray|brrip|bdrip|dvdrip|dvd|webrip|web-dl|webdl|web|hdtv|hdcam|cam|ts|tc|remux|hdlight|hdr10\+?|hdr|dolby\s?vision|dv|sdr)\b/i,
  // Video codecs
  /\b(?:x264|x265|h\.?264|h\.?265|hevc|avc|xvid|divx|av1|vp9|10\s?bits?|8\s?bits?)\b/i,
  // Audio
  /\b(?:ac-?3|eac-?3|e-ac-?3|dts(?:-?hd)?(?:\s?ma)?|truehd|atmos|aac|mp3|flac|opus|ddp?\s?\d(?:\.\d)?|dd\+?|5\.1|7\.1|2\.0)\b/i,
  // Language / subtitle tags
  /\b(?:multi|vostfr|vost|vff|vfq|vfi|vf2?|vo|truefrench|french|english|subfrench|fastsub)\b/i,
  // Release groups and site tags left as bracketed or trailing chunks
  /\[[^\]]*\]/g,
  /\{[^}]*\}/g,
  /\([^)]*\b(?:2160p?|1080p?|720p?|x26[45]|web|bluray)\b[^)]*\)/gi,
];

/** A 4-digit year, kept only when it is not the whole title (e.g. "1917"). */
const YEAR = /\b(?:19|20)\d{2}\b/;

/**
 * niceNameFromPath extracts a readable title from a file path.
 * Returns "" for an empty or extension-only input, so callers can fall back.
 */
export function niceNameFromPath(path: string | null | undefined): string {
  if (!path) return "";
  // Basename, on either separator - paths come from the server (POSIX) but a
  // Windows-mounted library can produce backslashes.
  const base = String(path).split(/[/\\]/).pop() ?? "";
  // Drop the extension, but only a real one (2-4 chars): "Rocket 2" must not
  // lose " 2", and "S.W.A.T" must not lose its last letter.
  let name = base.replace(/\.[A-Za-z0-9]{2,4}$/, "");

  // Underscores BEFORE the token pass: in a regex `_` counts as a word
  // character, so `\b` never fires inside "The_Big_Movie_1080p" and the
  // resolution tag survives. Dots are left alone here - the codec patterns
  // ("h.264", "dd5.1") need them - and are separated further down.
  name = name.replace(/_+/g, " ");

  for (const re of TOKEN_PATTERNS) {
    name = name.replace(new RegExp(re.source, re.flags.includes("g") ? re.flags : re.flags + "g"), " ");
  }

  // Separators to spaces, then collapse. Dots are separators in release naming;
  // hyphens too, but only when spaced or repeated, so "Spider-Man" survives.
  name = name.replace(/\.+/g, " ").replace(/\s+-\s+|-{2,}/g, " ");
  // A trailing "-GROUP" (release group). Restricted to an ALL-CAPS token: the
  // permissive version of this rule turned "Spider-Man" into "Spider", which is
  // precisely the over-stripping this helper must never do. A hyphenated word
  // in a real title ("Spider-Man", "Ant-Man") is capitalised, not shouted.
  name = name.replace(/-[A-Z0-9]{2,}\s*$/, " ");
  // Leftover punctuation from stripped tokens, and empty brackets.
  name = name.replace(/\(\s*\)|\[\s*\]|\{\s*\}/g, " ").replace(/\s+/g, " ").trim();
  // Trailing separators left by a stripped tail ("Rocket 2 -" / "Rocket 2 ,").
  name = name.replace(/[\s\-–—_,;:]+$/, "").replace(/^[\s\-–—_,;:]+/, "").trim();

  // A title reduced to nothing but a year is almost certainly a real title
  // ("1917", "2012"); recover it rather than returning empty.
  if (!name) {
    const y = base.match(YEAR);
    return y ? y[0] : "";
  }
  return name;
}
