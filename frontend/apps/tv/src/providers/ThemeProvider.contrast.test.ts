import { describe, it, expect } from "vitest";
import { themes } from "@evey/theme";

// WCAG 2.1 relative luminance, then the contrast ratio between two sRGB hexes.
// Mirrors packages/theme/src/contrast.test.ts's pinned arithmetic (that file
// covers the web/mobile CSS var layer; this one asserts the same guarantee
// for the exact tokens the TV app's ThemeProvider hands to useTheme()).
function luminance(hex: string): number {
  const h = hex.replace("#", "");
  const channels = [0, 2, 4].map((i) => parseInt(h.slice(i, i + 2), 16) / 255);
  const [r, g, b] = channels.map((v) => (v <= 0.03928 ? v / 12.92 : ((v + 0.055) / 1.055) ** 2.4));
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

function ratio(a: string, b: string): number {
  const [hi, lo] = [luminance(a), luminance(b)].sort((x, y) => y - x);
  return (hi + 0.05) / (lo + 0.05);
}

// AA for body text (4.5:1). The TV app renders onAccent labels at 18-24px,
// below the "large text" 24px/19px-bold threshold in several places (e.g.
// ScopeTab, GenreChip), so the large-text 3:1 allowance is not assumed here.
const AA = 4.5;

describe("TV theme contrast (WCAG AA) - onAccent on accent", () => {
  for (const [name, tokens] of Object.entries(themes)) {
    it(`${name}: renders onAccent text legibly on the accent fill`, () => {
      expect(ratio(tokens.colors.onAccent, tokens.colors.accent)).toBeGreaterThanOrEqual(AA);
    });
  }
});
