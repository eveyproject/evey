import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, existsSync } from "node:fs";
import { join, relative } from "node:path";

import * as fetcher from "./fetcher";
import * as watchHistoryLabel from "./watchHistoryLabel";
import * as audioChannelLayout from "./audioChannelLayout";
import * as notificationLabel from "./notificationLabel";
import * as format from "./format";
import * as seekIndexHelpers from "./seekIndexHelpers";
import * as displayTitle from "./displayTitle";
import * as personDepartment from "./personDepartment";
import * as pagination from "./pagination";
import * as exportPaging from "./exportPaging";
import * as regionDefaults from "./regionDefaults";
import * as libraryRules from "./libraryRules";

// Static guard: a test that mocks @evey/api wholesale must not hide the shared
// helpers the file under test imports from it.
//
// `vi.mock("@evey/api", () => ({ ... }))` REPLACES the module. Listing the
// endpoints a screen calls is the obvious thing to write, and it works right
// up to the day that screen also imports a helper - collectAllPages,
// formatBytes, hasMoreFromCursor. The helper is then `undefined` at runtime,
// and what happens next depends entirely on whether the test exercises that
// line: usually it does not, so the suite stays green while the mock has
// quietly stopped resembling the real module. When it does, the failure points
// at the component, not at the mock.
//
// This cost six separate fixes in one night (2026-08-08) across web, mobile and
// TV, always found by hand and always after the fact. The cure is one word:
//
//   vi.mock("@evey/api", async (importOriginal) => ({
//     ...(await importOriginal<typeof import("@evey/api")>()),
//     someEndpoint: vi.fn(),
//   }));
//
// The guard is deliberately narrow. It does not object to mocking the package,
// nor to omitting endpoints - only to omitting a HAND-WRITTEN helper that the
// file under test actually imports. Endpoints are generated and are what a mock
// is normally for; helpers are shared logic that a mock has no business
// replacing with `undefined`.

// The hand-written modules re-exported by index.ts. Imported for real rather
// than name-listed, so a helper added to any of them is covered the same day it
// lands, with no list to remember to update.
const SHARED_MODULES = {
  fetcher,
  watchHistoryLabel,
  audioChannelLayout,
  notificationLabel,
  format,
  seekIndexHelpers,
  displayTitle,
  personDepartment,
  pagination,
  exportPaging,
  regionDefaults,
  libraryRules,
};

const SHARED_NAMES = new Set(Object.values(SHARED_MODULES).flatMap((m) => Object.keys(m)));

const FRONTEND_DIR = join(__dirname, "..", "..", "..");
const ROOTS = [join(FRONTEND_DIR, "apps"), join(FRONTEND_DIR, "packages")];
const SKIP_DIRS = new Set(["node_modules", "dist", "build", ".expo", "android", "ios", "generated", ".next"]);

function testFiles(dir: string, out: string[] = []): string[] {
  for (const entry of readdirSync(dir, { withFileTypes: true })) {
    const full = join(dir, entry.name);
    if (entry.isDirectory()) {
      if (!SKIP_DIRS.has(entry.name)) testFiles(full, out);
    } else if (/\.test\.tsx?$/.test(entry.name)) {
      out.push(full);
    }
  }
  return out;
}

/** The factory body of the `vi.mock("@evey/api", ...)` call, or null. */
function eveyApiMockBody(source: string): string | null {
  const at = source.search(/vi\.mock\(\s*"@evey\/api"/);
  if (at < 0) return null;
  const end = source.indexOf("\n}));", at);
  return source.slice(at, end < 0 ? source.length : end);
}

/** Named identifiers a file imports from @evey/api. */
function importedFromEveyApi(source: string): Set<string> {
  const names = new Set<string>();
  const re = /import\s+(?:type\s+)?\{([^}]*)\}\s*from\s*"@evey\/api"/gs;
  for (const match of source.matchAll(re)) {
    for (const part of match[1].split(",")) {
      const name = part.trim().replace(/^type\s+/, "").split(/\s+as\s+/)[0].trim();
      if (name) names.add(name);
    }
  }
  return names;
}

function sourceUnderTest(testPath: string): string | null {
  const base = testPath.replace(/\.test\.tsx?$/, "");
  for (const ext of [".ts", ".tsx"]) {
    if (existsSync(base + ext)) return base + ext;
  }
  return null;
}

describe("mocks of @evey/api keep the shape of the real module", () => {
  it("never hides a shared helper the file under test imports", () => {
    const offenders: string[] = [];

    for (const root of ROOTS) {
      for (const testPath of testFiles(root)) {
        const body = eveyApiMockBody(readFileSync(testPath, "utf8"));
        if (body === null || body.includes("importOriginal")) continue;

        const source = sourceUnderTest(testPath);
        if (!source) continue;

        const needed = [...importedFromEveyApi(readFileSync(source, "utf8"))].filter((n) => SHARED_NAMES.has(n));
        // A mock may stand in for a helper explicitly; only silence is a bug.
        const hidden = needed.filter((n) => !new RegExp(`\\b${n}\\s*[:,]`).test(body));
        if (hidden.length > 0) {
          offenders.push(`${relative(FRONTEND_DIR, testPath)} hides ${hidden.sort().join(", ")}`);
        }
      }
    }

    expect(
      offenders,
      "spread the real module in these mocks:\n" +
        "  vi.mock(\"@evey/api\", async (importOriginal) => ({ ...(await importOriginal<typeof import(\"@evey/api\")>()), ... }))\n" +
        offenders.map((o) => `  - ${o}`).join("\n"),
    ).toEqual([]);
  });

  // If index.ts grows a hand-written module that is not imported above, the
  // guard silently stops covering its helpers. Comparing against index.ts is
  // what keeps the list honest.
  it("covers every hand-written module re-exported by index.ts", () => {
    const index = readFileSync(join(__dirname, "index.ts"), "utf8");
    const reExported = [...index.matchAll(/export \* from "\.\/([a-zA-Z][a-zA-Z0-9]*)"/g)].map((m) => m[1]);
    expect(reExported.sort()).toEqual(Object.keys(SHARED_MODULES).sort());
  });
});
