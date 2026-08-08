import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync } from "node:fs";
import { join, relative } from "node:path";

// Static guard: a parent-scoped ListMediaItems call must state its page size.
//
// ListMediaItems answers with the handler's default - 50 - when the caller
// sends no pagination (clampPagination, grpcapi/library_handler_mediaitems.go).
// For a library grid that is fine: the user scrolls and more arrives. For a
// parentId query it is not, because those lists are consumed whole:
//
//   - Episodes have NO server-side sort key. SortColumnMap covers title,
//     added_at, year, premiere_date and community_rating, not the episode
//     index, so the server answers in sort_title order and every client
//     re-sorts by index itself. That is only correct while the client holds the
//     entire season. Past 50 episodes the page held the alphabetically-first
//     50, so sorting them and taking [0] - which is exactly what the Play
//     button does - started playback on whichever episode sorted first by
//     TITLE.
//   - The counts these screens render ("12 episodes", "3 seasons") are
//     items.length, so a capped page becomes a wrong TOTAL rather than a short
//     list.
//
// Three screens shipped that way at once (the web series detail, the web player
// picker, the TV media screen), and the reason none of them looked wrong in
// review is the point of this guard: **an omitted page size is
// indistinguishable from a forgotten one**. A number, even the same number the
// server would have used, says someone decided.
//
// The rule is deliberately narrow. It says nothing about page sizes for library
// grids, search, or any other query - only that a parentId call must name one.
// The client_page_size guard covers the opposite mistake (asking for more than
// the server will serve).

const FRONTEND_DIR = join(__dirname, "..", "..", "..");
const APPS_DIR = join(FRONTEND_DIR, "apps");
const SKIP_DIRS = new Set(["node_modules", "dist", "build", ".expo", "android", "ios", "generated", ".next"]);

// Both the hook and the imperative form of the generated client.
const LIST_CALL = /(?:use)?[lL]ibraryServiceListMediaItems\s*\(\s*(\{[\s\S]*?\})/g;
const PAGE_SIZE_KEY = /"pagination\.pageSize"\s*:/;

function sourceFiles(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) sourceFiles(full, out);
    } else if (/\.tsx?$/.test(entry.name) && !/\.test\.tsx?$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

describe("parent-scoped media list queries state their page size", () => {
  it("no parentId query relies on the server default", () => {
    const offenders: string[] = [];

    for (const file of sourceFiles(APPS_DIR)) {
      const source = readFileSync(file, "utf8");
      for (const match of source.matchAll(LIST_CALL)) {
        const args = match[1];
        if (!args.includes("parentId")) continue;
        if (PAGE_SIZE_KEY.test(args)) continue;
        const line = source.slice(0, match.index).split("\n").length;
        offenders.push(`${relative(FRONTEND_DIR, file)}:${line}`);
      }
    }

    expect(
      offenders,
      "these parentId queries fall back to the handler's default of 50, which for a season " +
        "means an alphabetically-truncated episode list and a Play button that can start on the " +
        "wrong episode. Add an explicit \"pagination.pageSize\":\n" +
        offenders.map((o) => `  - ${o}`).join("\n"),
    ).toEqual([]);
  });

  // A rename of the generated client would make the scan silently match
  // nothing, and a guard that inspects zero call sites passes forever.
  it("actually finds the call sites it claims to check", () => {
    let seen = 0;
    for (const file of sourceFiles(APPS_DIR)) {
      for (const match of readFileSync(file, "utf8").matchAll(LIST_CALL)) {
        if (match[1].includes("parentId")) seen++;
      }
    }
    expect(seen, "no parentId ListMediaItems call found - has the client been renamed?").toBeGreaterThan(5);
  });
});
