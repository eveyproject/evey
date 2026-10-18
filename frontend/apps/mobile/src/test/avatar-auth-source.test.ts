import { describe, expect, it } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join } from "node:path";

// Profile avatars are fetched with the access token as an Authorization
// header. Six phone screens put it in the URL instead (?token=), where it lands
// in the image cache, the proxy's access log and any crash report quoting the
// URL. Static, because the regression is a string in a source file: every
// screen that builds an avatar URL must pass the header, and none may put the
// token back in the query.
//
// The avatar and artwork routes are public on the server, so no client needs a
// token on them at all: the web and the browser television used to add one
// anyway. The leak check runs over every client and the shared package.

const APPS = join(__dirname, "..", "..", "..");
const ROOTS = ["mobile/app", "mobile/src", "tv/app", "tv/src"].map((r) => join(APPS, r));
const ALL_CLIENTS = [...ROOTS, ...["web/src", "tv-web/src", "../packages/client/src"].map((r) => join(APPS, r))];

function sources(dir: string): string[] {
  return readdirSync(dir).flatMap((name) => {
    const path = join(dir, name);
    if (statSync(path).isDirectory()) return name === "node_modules" ? [] : sources(path);
    return /\.tsx?$/.test(name) && !/\.test\.tsx?$/.test(name) ? [path] : [];
  });
}

const read = (path: string) => ({ path, text: readFileSync(path, "utf8") });
const files = ROOTS.flatMap(sources).map(read);
const avatarFiles = files.filter((f) => /\/api\/v1\/profiles\/\$\{[^}]+\}\/avatar[`?]/.test(f.text));

// A line that names a public image route - its URL template, or a helper that
// builds one - and carries a token on that same line.
const PUBLIC_IMAGE = /\/artwork\/(?:\$\{|poster|backdrop)|\/avatar[`?]|\b(?:artworkUrl|posterSrc|avatarSrc)\(/;
const TOKEN = /token=|tokenParam|tokenInQuery|authedSrc\(|accessToken|\btoken\b/i;
const publicImageTokenLines = ALL_CLIENTS.flatMap(sources).map(read).flatMap((f) =>
  f.text.split("\n").flatMap((line, i) =>
    PUBLIC_IMAGE.test(line) && !/\/select\b/.test(line) && TOKEN.test(line) ? [`${f.path}:${i + 1}`] : [],
  ),
);

describe("profile avatar requests", () => {
  it("found the screens that draw an avatar", () => {
    expect(avatarFiles.length).toBeGreaterThanOrEqual(8);
  });

  it("never carry the access token in the URL", () => {
    const leaking = files.filter((f) => /\/avatar\?token=/.test(f.text)).map((f) => f.path);
    expect(leaking).toEqual([]);
  });

  it("are not handed a token by any client, nor are artwork requests", () => {
    expect(publicImageTokenLines).toEqual([]);
  });

  it("pass it as a header wherever an avatar is drawn", () => {
    const withoutHeader = avatarFiles.filter((f) => !/headers: authHeaders/.test(f.text)).map((f) => f.path);
    expect(withoutHeader).toEqual([]);
  });
});
