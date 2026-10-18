import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

// A url the server hands over may be another host's: a collection poster is a
// TMDB url, a music cover can come from a cover service. Gluing the stream
// token onto it sent the viewer's JWT to that host. Server images go through
// serverImageUri (native) or serverImageSrc (tv-web), which add the token to
// the server's own relative paths only. Static, because the regression is a
// string concatenation in a screen.

const APPS = join(__dirname, "..", "..", "..");
const ROOTS = ["mobile/app", "mobile/src", "tv/app", "tv/src", "tv-web/src", "web/src"].map((r) => join(APPS, r));

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "node_modules" ? [] : sources(path);
    return /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name) ? [path] : [];
  });
}

describe("server-provided image urls", () => {
  it("never get the token glued on as they come", () => {
    const glued = /\$\{(String\()?[\w.?]*(Url|Uri)\)?\}\$\{tokenParam\}/;
    const offenders = ROOTS.flatMap(sources).filter((p) => glued.test(readFileSync(p, "utf8")));
    expect(offenders).toEqual([]);
  });
});
