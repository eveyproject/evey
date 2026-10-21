import { describe, it, expect, afterEach } from "vitest";
import { applyThemeToDOM } from "./index";

// A <link id="theme-link"> that counts how often its href is assigned: each
// assignment is a new fetch of the sheet in the browser.
function fakeLink(href: string, media = "") {
  let current = href;
  let currentMedia = media;
  const link = {
    sets: 0,
    mediaSets: 0,
    get media() { return currentMedia; },
    set media(v: string) { link.mediaSets++; currentMedia = v; },
    getAttribute: (name: string) => (name === "href" ? current : null),
    get href() { return current; },
    set href(v: string) { link.sets++; current = v; },
  };
  return link;
}

function withLink(link: ReturnType<typeof fakeLink>) {
  (globalThis as { document?: unknown }).document = { getElementById: (id: string) => (id === "theme-link" ? link : null) };
}

afterEach(() => { delete (globalThis as { document?: unknown }).document; });

describe("applyThemeToDOM", () => {
  it("leaves the link alone when it already points at the theme, so the sheet is not fetched again", () => {
    const link = fakeLink("/themes/default.css");
    withLink(link);
    applyThemeToDOM("default");
    applyThemeToDOM("default");
    expect(link.sets).toBe(0);
    // No media attribute already means all media: rewriting it re-evaluates the sheet.
    expect(link.mediaSets).toBe(0);
  });

  it("restores a link left on another media", () => {
    const link = fakeLink("/themes/default.css", "print");
    withLink(link);
    applyThemeToDOM("default");
    expect(link.media).toBe("all");
  });

  it("points the link at another theme once", () => {
    const link = fakeLink("/themes/default.css");
    withLink(link);
    applyThemeToDOM("oled");
    applyThemeToDOM("oled");
    expect(link.sets).toBe(1);
    expect(link.href).toBe("/themes/oled.css");
  });
});
