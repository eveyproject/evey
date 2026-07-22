import { describe, it, expect } from "vitest";
import { themeVars } from "./themeVars";

// WCAG 2.1 relative luminance, then the contrast ratio between two sRGB hexes.
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

// AA for body text. The accent is used at text-xs/text-sm in ~50 places
// (including the login page's "forgot password" link), so the large-text 3:1
// allowance does not apply to it.
const AA = 4.5;

const isHex = (v: string | undefined): v is string => typeof v === "string" && /^#[0-9a-fA-F]{6}$/.test(v);

// Every pair here was failing at some point before 2026-07-23: the light themes
// rendered accent-coloured links at 3.33:1 and 4.24:1, and four of the five
// themes put white on an accent-filled button - amoled at 1.85:1, which is
// barely legible. A colour token is easy to nudge for looks and impossible to
// eyeball for contrast, so the arithmetic is pinned here rather than audited.
describe("theme contrast (WCAG AA)", () => {
  for (const [name, vars] of Object.entries(themeVars)) {
    const accent = vars["--accent"];
    const bg = vars["--bg"];
    const surface = vars["--surface-raised"];
    const onAccent = vars["--filter-active-text"];
    const accentHover = vars["--accent-hover"];

    if (!isHex(accent) || !isHex(bg)) continue;

    describe(name, () => {
      it("renders accent-coloured text legibly on the page background", () => {
        expect(ratio(accent, bg)).toBeGreaterThanOrEqual(AA);
      });

      if (isHex(surface)) {
        it("renders accent-coloured text legibly on a raised surface", () => {
          expect(ratio(accent, surface)).toBeGreaterThanOrEqual(AA);
        });
      }

      if (isHex(onAccent)) {
        it("renders button text legibly on an accent fill", () => {
          expect(ratio(onAccent, accent)).toBeGreaterThanOrEqual(AA);
        });

        if (isHex(accentHover)) {
          // Hovering must not be the moment the label becomes unreadable.
          it("keeps button text legible on the accent hover fill", () => {
            expect(ratio(onAccent, accentHover)).toBeGreaterThanOrEqual(AA);
          });
        }
      }
    });
  }
});
