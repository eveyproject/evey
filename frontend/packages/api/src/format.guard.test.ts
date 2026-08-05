import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Anti-drift ratchet for the consolidation of 2026-08-05.
//
// The byte/clock/duration formatters had grown ~26 per-screen copies across
// the three apps, and they disagreed: admin dashboards divided by 1e9 while
// every other screen used 1024, the loadtest panel hardcoded French unit
// labels, two web formatDuration took different units (ms vs seconds), and
// null/0 rendered three different ways for the same stat. Each copy compiled
// and each app's tests passed against its own copy - only a scan for the
// DECLARATIONS catches the next copy being written.
const APPS = join(__dirname, "..", "..", "..", "apps");

// Declaration of one of the consolidated formatters, as a function or an
// arrow const. formatMbps/formatBps/formatDate stay local on purpose: they
// were never duplicated.
const FORMATTER_DECL =
  /(?:function|const)\s+format(?:Bytes|Size|FileSize|TimeMs|Time|DurationMs|DurationSec|Duration|Ms)\s*[(=]/;

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

describe("byte/clock/duration formatters live in exactly one place", () => {
  const files = sourceFiles(APPS);

  it("scans a non-trivial number of app sources", () => {
    // Without this the suite passes vacuously the day the apps move.
    expect(files.length).toBeGreaterThan(200);
  });

  it("no app re-declares a formatter", () => {
    const offenders = files
      .filter((f) => FORMATTER_DECL.test(readFileSync(f, "utf8")))
      .map((f) => relative(APPS, f));
    expect(
      offenders,
      `these declare their own formatter - import formatBytes/formatTime/formatDurationMs/formatDurationSec from "@evey/api" instead:\n${offenders.join("\n")}`,
    ).toEqual([]);
  });
});
