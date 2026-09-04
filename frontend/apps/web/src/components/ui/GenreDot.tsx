import { genreDotColor } from "@evey/api";

/**
 * The coloured dot that identifies a genre, wherever a genre is named.
 *
 * A dot rather than a tinted chip: the label keeps the theme's own text colour,
 * so no hash can ever produce an unreadable genre. The ring is currentColor -
 * the chip's text colour, which is readable against its background by
 * construction - so the dot stays visible on a plain surface and on the accent
 * fill of a selected chip alike, without this component knowing which it is on.
 *
 * aria-hidden: the colour adds nothing a screen reader can use, and the genre's
 * name is right next to it.
 */
export function GenreDot({ label }: { label: string }) {
  return (
    <span
      aria-hidden="true"
      className="inline-block rounded-full shrink-0"
      style={{
        width: 8,
        height: 8,
        backgroundColor: genreDotColor(label),
        boxShadow: "0 0 0 1px currentColor",
      }}
    />
  );
}
