/**
 * A stable colour per genre, for the dot shown next to its name.
 *
 * Derived from the label rather than looked up in a table. A table would have to
 * be maintained in six languages, would leave every genre a provider invents
 * uncoloured, and would need a decision about which hue "means" Thriller - a
 * decision with no right answer. Hashing gives every genre a colour, the same
 * one on every screen and every reload, for free.
 *
 * This only works because genres are normalised server-side first: "Drama" and
 * "Drame" now arrive as one label, so they get one colour. Before that fix the
 * same genre would have been two chips in two colours, which would have made the
 * duplication look deliberate.
 *
 * Only the HUE comes from the hash. Saturation and lightness are fixed, which is
 * what keeps every genre's dot equally visible - the failure mode of naive
 * hash-to-colour is that some inputs land on near-white or near-black. It also
 * means the dot needs no knowledge of the active theme: a mid-saturation colour
 * at 55% lightness reads on both light and dark surfaces, and the LABEL keeps
 * the theme's own text colour, so contrast is never this function's problem.
 */

/** FNV-1a: short, dependency-free, and well spread over small strings. */
function hashLabel(label: string): number {
  let h = 0x811c9dc5;
  const s = label.trim().toLowerCase();
  for (let i = 0; i < s.length; i++) {
    h ^= s.charCodeAt(i);
    h = Math.imul(h, 0x01000193) >>> 0;
  }
  return h;
}

/** genreHue returns the 0-359 hue for a genre label. */
export function genreHue(label: string): number {
  return hashLabel(label) % 360;
}

/**
 * genreDotColor returns the CSS colour of a genre's dot. Theme-independent by
 * design - see the note above.
 */
export function genreDotColor(label: string): string {
  return `hsl(${genreHue(label)} 65% 55%)`;
}
