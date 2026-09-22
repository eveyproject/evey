import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Which clients get a LONG session, and which do not.
//
// The server grants 180 days to a client that asks with X-Persistent-Session
// and 7 otherwise. The rule the owner stated on 2026-09-23: a television
// keeps its session for months, because there is no keyboard to answer a
// login screen with; a browser on a computer keeps the short one.
//
// Only initFetcher (this package) sends that header, so "which clients ask"
// is exactly "which clients wire initFetcher". This guard pins that list:
// wiring it into the browser application, or unwiring it from a television,
// is a change to the session policy and must be a red test, not a surprise
// a week later.
const APPS = join(__dirname, "..", "..", "..", "..", "apps");

const ASK_FOR_A_LONG_SESSION = ["mobile", "tv", "tv-web"];
const KEEP_THE_SHORT_ONE = ["web"];

function sourceFiles(dir: string): string[] {
  let out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === "dist" || entry === "build" || entry === ".expo") continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out = out.concat(sourceFiles(full));
    else if (/\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry)) out.push(full);
  }
  return out;
}

function asksForALongSession(app: string): string[] {
  const dir = join(APPS, app);
  return sourceFiles(dir)
    .filter((f) => /initFetcher\s*\(|X-Persistent-Session/.test(readFileSync(f, "utf8")))
    .map((f) => relative(APPS, f));
}

describe("session lifetime is a deliberate choice per client", () => {
  it("scans the applications, so the guard cannot pass vacuously", () => {
    expect(sourceFiles(APPS).length).toBeGreaterThan(200);
  });

  it.each(ASK_FOR_A_LONG_SESSION)("%s asks for a long session", (app) => {
    expect(asksForALongSession(app)).not.toEqual([]);
  });

  it.each(KEEP_THE_SHORT_ONE)("%s does not", (app) => {
    expect(asksForALongSession(app)).toEqual([]);
  });
});
