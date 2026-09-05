import { describe, it, expect } from "vitest";
import { themeVars } from "./themeVars";

// WCAG 2.1 relative luminance, then the contrast ratio between two sRGB hexes.
function luminance(hex: string): number {
  const raw = hex.replace("#", "");
  // Expand the 3-digit form, and refuse anything else outright. This used to
  // slice past the end of a short hex and produce NaN, which every comparison
  // then silently reported as "not >= 4.5" - an assertion that looks like it
  // failed on contrast when it actually never ran.
  const h = raw.length === 3 ? raw.split("").map((c) => c + c).join("") : raw;
  if (!/^[0-9a-fA-F]{6}$/.test(h)) {
    throw new Error(`luminance: not a colour this test can read: ${JSON.stringify(hex)}`);
  }
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
    const onDanger = vars["--on-danger"];
    const dangerBg = vars["--danger-bg"];
    const colorDanger = vars["--color-danger"];
    const toastError = vars["--toast-error-icon"];

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

      // The three text ramps on the three surfaces. All 45 pairs already pass
      // (5.03:1 at the tightest, light-blue's muted on its raised surface), so
      // this is a tripwire rather than a fix: nothing asserted them, and that
      // is precisely how the error tint drifted to 3.30:1 unnoticed. Secondary
      // and muted carry real content - dates, counts, episode subtitles - not
      // decoration, so AA applies to them exactly as to the primary ramp.
      for (const [ramp, fg] of [
        ["primary text", vars["--text"]],
        ["secondary text", vars["--text-secondary"]],
        ["muted text", vars["--text-muted"]],
      ] as const) {
        if (!isHex(fg)) continue;
        for (const [where, value] of [
          ["page background", bg],
          ["surface", vars["--surface"]],
          ["raised surface", surface],
        ] as const) {
          if (!isHex(value)) continue;
          it(`reads ${ramp} on the ${where}`, () => {
            expect(ratio(fg, value)).toBeGreaterThanOrEqual(AA);
          });
        }
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

      // The danger surfaces carry their own foreground (--on-danger), not the
      // accent one. This was the exact blind spot that let a regression through:
      // flipping --filter-active-text dark to fix accent contrast silently broke
      // the delete button and error toast, because nothing asserted their
      // contrast. Pin them so that can never recur.
      if (isHex(onDanger)) {
        if (isHex(dangerBg)) {
          it("renders text legibly on the danger button fill", () => {
            expect(ratio(onDanger, dangerBg)).toBeGreaterThanOrEqual(AA);
          });
        }
        // --color-danger (aliased as --danger) is a DIFFERENT value from
        // --danger-bg on some themes and is what destructive buttons/icons
        // painted white-on-red actually use. This is the exact pair that was
        // missing: --danger-bg passed AA while --color-danger silently did
        // not (#ef4444 at 3.76:1 on default/amoled), and nothing caught it.
        if (isHex(colorDanger)) {
          it("renders text legibly on the --color-danger fill", () => {
            expect(ratio(onDanger, colorDanger)).toBeGreaterThanOrEqual(AA);
          });
        }
        if (isHex(toastError)) {
          it("renders text legibly on the error toast fill", () => {
            expect(ratio(onDanger, toastError)).toBeGreaterThanOrEqual(AA);
          });
        }
      }
    });
  }
});

// The derived onSuccess/onWarning tokens (tokens.ts computes best-of dark or
// white against the fill) must actually clear AA on every theme - if a theme
// ever ships a mid-luminance success/warning where NEITHER side reads, this
// is the tripwire.
import { deriveTokens } from "./tokens";
import { THEME_NAMES } from "./themeVars";

describe("derived on-colour tokens (mobile/TV chips)", () => {
  for (const name of THEME_NAMES) {
    const t = deriveTokens(name);
    it(`${name}: text reads on the success fill`, () => {
      expect(ratio(t.colors.onSuccess, t.colors.success)).toBeGreaterThanOrEqual(AA);
    });
    it(`${name}: text reads on the warning fill`, () => {
      expect(ratio(t.colors.onWarning, t.colors.warning)).toBeGreaterThanOrEqual(AA);
    });
    // `error` is a mobile-only tint, SEPARATE from --danger-bg. The assertions
    // above and the --on-danger one earlier both look at danger, so nothing
    // covered this fill: the destructive buttons of the mobile admin screens
    // painted plain white on it and failed AA on 4 of the 5 themes (2.77:1 to
    // 3.76:1). A token is only safe once something asserts the pair it is used
    // in - not the pair it resembles.
    it(`${name}: text reads on the mobile error fill`, () => {
      expect(ratio(t.colors.onError, t.colors.error)).toBeGreaterThanOrEqual(AA);
    });
    // ...and the other direction, which is the one that was missing. `error` is
    // a FOREGROUND far more often than a fill (~57 of its uses paint text or an
    // icon, three fill a surface), yet only the fill was asserted. Both light
    // themes failed here: #ef4444 read at 3.30:1 on light-default's raised
    // surface and #dc2626 at 3.92:1 on light-blue's - an error message that the
    // viewer who most needs to read it cannot.
    //
    // All three surfaces, because a destructive row sits on any of them.
    for (const [surface, value] of [
      ["page background", t.colors.bg],
      ["surface", t.colors.surface],
      ["raised surface", t.colors.surfaceRaised],
    ] as const) {
      it(`${name}: error-coloured text reads on the ${surface}`, () => {
        expect(ratio(t.colors.error, value)).toBeGreaterThanOrEqual(AA);
      });
    }
    // onDanger against `danger` (--color-danger) is asserted above, per theme,
    // against the raw CSS var - not repeated here against the derived token,
    // since tokens.colors.danger is the same value via resolveVar().
  }
});
