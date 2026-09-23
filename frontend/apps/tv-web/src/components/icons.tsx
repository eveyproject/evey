/**
 * Glyphs drawn rather than typed. 👍 👎 ⏮ ⏭ are emoji: only a colour-emoji
 * font draws them, and many television browsers ship none - the title page
 * showed two empty boxes where the thumbs were. An inline path needs no font,
 * and takes the text colour, so a focused button still paints it right.
 */
const box = { width: "1.2em", height: "1.2em", verticalAlign: "-0.2em" } as const;

function Glyph({ d }: { d: string }) {
  return (
    <svg viewBox="0 0 24 24" style={box} aria-hidden="true" focusable="false">
      <path d={d} fill="currentColor" />
    </svg>
  );
}

export const ThumbUpIcon = () => (
  <Glyph d="M2 21h4V9H2v12zm20-11c0-1.1-.9-2-2-2h-6.3l.95-4.57.03-.32c0-.41-.17-.79-.44-1.06L13.17 1 6.59 7.59C6.22 7.95 6 8.45 6 9v10c0 1.1.9 2 2 2h9c.83 0 1.54-.5 1.84-1.22l3.02-7.05c.09-.23.14-.47.14-.73v-2z" />
);

export const ThumbDownIcon = () => (
  <Glyph d="M22 3h-4v12h4V3zM2 14c0 1.1.9 2 2 2h6.3l-.95 4.57-.03.32c0 .41.17.79.44 1.06L10.83 23l6.58-6.59c.37-.36.59-.86.59-1.41V5c0-1.1-.9-2-2-2H7c-.83 0-1.54.5-1.84 1.22L2.14 11.27C2.05 11.5 2 11.74 2 12v2z" />
);

export const PreviousIcon = () => <Glyph d="M6 6h2v12H6V6zm3.5 6l8.5 6V6l-8.5 6z" />;

export const NextIcon = () => <Glyph d="M16 6h2v12h-2V6zM6 18l8.5-6L6 6v12z" />;
