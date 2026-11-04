import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Every home rail draws the same card from feedItemToCard. The popular rail
// had a hand-written projection in each app ({ id: i.mediaItemId, ... }),
// which kept only the title and the year: no poster placeholder, no score.
// Each copy compiled and passed its app's tests, so only a scan of the app
// sources catches the next one.
const APPS = join(__dirname, "..", "..", "..", "..", "apps");

function sourceFiles(dir: string): string[] {
  let out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === "dist" || entry === "build" || entry === ".expo" || entry === "android" || entry === "ios") continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) out = out.concat(sourceFiles(full));
    else if (/\.tsx?$/.test(entry) && !/\.test\.tsx?$/.test(entry)) out.push(full);
  }
  return out;
}

// A local projection of a feed or popular item onto a card.
const LOCAL_PROJECTION = /\bid:\s*\w+\.mediaItemId\b/;

describe("home rails project their items through feedItemToCard", () => {
  const files = sourceFiles(APPS);
  const railFiles = files.filter((f) => readFileSync(f, "utf8").includes("useMostPopular("));

  it("finds the popular rail of every app", () => {
    // Without this the suite passes vacuously the day the rails move.
    expect(railFiles.length).toBeGreaterThanOrEqual(4);
  });

  it("no popular rail builds its own card", () => {
    const offenders = railFiles
      .filter((f) => LOCAL_PROJECTION.test(readFileSync(f, "utf8")))
      .map((f) => relative(APPS, f));
    expect(offenders, `project the items with feedItemToCard / feedItemsToCards from "@evey/client":\n${offenders.join("\n")}`).toEqual([]);
  });
});
