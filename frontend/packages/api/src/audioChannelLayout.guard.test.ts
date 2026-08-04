import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Anti-drift ratchet for the consolidation of 2026-08-04.
//
// The channel-layout table had grown six copies (web AudioTrackSelector +
// StreamInfoPanel, mobile audioTrackLabel + StreamInfoPanel, TV
// AudioTrackPicker + StreamInfoPanel) and they no longer agreed: web's track
// menu rendered "2.0" while web's own stream-info panel rendered "Stereo" for
// the same track on the same screen, and five of the six hardcoded English
// words into a UI that ships in six languages.
//
// Nothing about that is visible to the type-checker: each copy compiled, each
// app's tests passed against its own copy. Only a check on the DATA - a
// mapping from the channel count 6 to the string "5.1" - catches the seventh
// copy being written.
const APPS = join(__dirname, "..", "..", "..", "apps");

// The three shapes the copies actually took, found by writing this guard too
// narrowly first: a table (`6: "5.1"`), an if-chain (`ch === 6) return "5.1"`)
// and a switch (`case 6:` then `return "5.1"`). The switch one slipped through
// the first version and hid a seventh copy - in which `1` still returned
// "Mono" while `2` already returned "2.0", the drift inside a single function.
const LAYOUT_TABLE = /6\s*(?::|\)\s*return)\s*"5\.1"|case\s+6\s*:\s*(?:\/\/[^\n]*\n\s*)?return\s*"5\.1"/;

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

describe("audio channel layout lives in exactly one place", () => {
  const files = sourceFiles(APPS);

  it("scans a non-trivial number of app sources", () => {
    // Without this the suite passes vacuously the day the apps move.
    expect(files.length).toBeGreaterThan(200);
  });

  it("no app re-declares the layout table", () => {
    const offenders = files
      .filter((f) => LAYOUT_TABLE.test(readFileSync(f, "utf8")))
      .map((f) => relative(APPS, f));
    expect(
      offenders,
      `these declare their own channel-layout mapping - import formatChannelLayout from "@evey/api" instead:\n${offenders.join("\n")}`,
    ).toEqual([]);
  });
});
