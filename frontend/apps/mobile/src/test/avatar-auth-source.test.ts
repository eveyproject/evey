import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

// Profile avatars are fetched with the access token as an Authorization
// header. Six phone screens put it in the URL instead (?token=), where it lands
// in the image cache, the proxy's access log and any crash report quoting the
// URL. Static, because the regression is a string in a source file: every
// screen that builds an avatar URL must pass the header, and none may put the
// token back in the query.

const APPS = join(__dirname, "..", "..", "..");
const ROOTS = ["mobile/app", "mobile/src", "tv/app", "tv/src"].map((r) => join(APPS, r));

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "node_modules" ? [] : sources(path);
    return /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name) ? [path] : [];
  });
}

const files = ROOTS.flatMap(sources).map((path) => ({ path, text: readFileSync(path, "utf8") }));
const avatarFiles = files.filter((f) => /\/api\/v1\/profiles\/\$\{[^}]+\}\/avatar[`?]/.test(f.text));

describe("profile avatar requests", () => {
  it("found the screens that draw an avatar", () => {
    expect(avatarFiles.length).toBeGreaterThanOrEqual(8);
  });

  it("never carry the access token in the URL", () => {
    const leaking = files.filter((f) => /\/avatar\?token=/.test(f.text)).map((f) => f.path);
    expect(leaking).toEqual([]);
  });

  it("pass it as a header wherever an avatar is drawn", () => {
    const withoutHeader = avatarFiles.filter((f) => !/headers: authHeaders/.test(f.text)).map((f) => f.path);
    expect(withoutHeader).toEqual([]);
  });
});
