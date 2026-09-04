import { describe, expect, it } from "vitest";
import { readFileSync, readdirSync, statSync } from "node:fs";
import { join } from "node:path";

// getItemLayout receives the ROW index on a FlatList with numColumns > 1, not
// the item index. Dividing it by the column count divides twice: consecutive
// rows report the same offset, the list thinks they share a vertical position,
// and scrolling jumps a screenful at a time instead of flowing.
//
// It shipped in THREE screens at once (library, favorites, collection detail)
// because each one copied the previous, and it is invisible in review - the
// arithmetic looks like it is converting an item index to a row. A user had to
// report it: "j'ai pas un fil continu mais comme des pages" (2026-09-04).
//
// A static check is the right shape here: the bug is a source pattern, and
// asserting it through a rendered FlatList would need the virtualisation
// internals to misbehave in a test environment to catch anything.

const ROOTS = ["app", "src"];
const OFFENDING = /offset:\s*[A-Z_]*(?:ROW_)?HEIGHT\s*\*\s*Math\.floor\(\s*index\s*\/\s*[A-Z_]*COLUMNS/;

function walk(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules" || entry === ".expo") continue;
    const p = join(dir, entry);
    if (statSync(p).isDirectory()) walk(p, out);
    else if (/\.tsx?$/.test(p)) out.push(p);
  }
  return out;
}

describe("getItemLayout on multi-column lists", () => {
  it("never divides the row index by the column count", () => {
    const offenders: string[] = [];
    for (const root of ROOTS) {
      for (const file of walk(join(__dirname, "..", "..", root))) {
        const src = readFileSync(file, "utf8");
        if (OFFENDING.test(src)) offenders.push(file);
      }
    }
    expect(
      offenders,
      "getItemLayout gets the ROW index when numColumns > 1; dividing it again " +
        "makes several rows share one offset and the grid scrolls in jumps. " +
        "Use `offset: ROW_HEIGHT * index`.",
    ).toEqual([]);
  });
});
