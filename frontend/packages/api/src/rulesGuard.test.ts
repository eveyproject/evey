import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Static guard: no app may redeclare a shared library rule locally.
//
// Every rule in libraryRules.ts was, at some point, a per-app copy kept in sync
// by nothing but a "Mirrors web's X" comment. That failed exactly as you would
// expect: on 2026-07-28 mobile and TV had copied DEFAULT_TYPE_FILTER but not
// DEDICATED_ROUTES, so a music library opened an empty media grid on both, and
// the same never-populated `totalPages` stop condition shipped twice.
//
// A comment cannot fail a build. This can. It is deliberately narrow - it
// matches DECLARATIONS of specific named rules, not usage and not values, so
// importing them from @evey/api is untouched and the guard has no opinion on
// code it does not know about.
//
// Adding a rule here means: move it into libraryRules.ts, import it everywhere,
// and add its name below. Reverting to a local copy then fails this test with
// the file and line that reintroduced it.

const APPS_DIR = join(__dirname, "..", "..", "..", "apps");
const SKIP_DIRS = new Set(["node_modules", "dist", "build", ".expo", "android", "ios", "generated", ".next"]);
const SOURCE_EXT = /\.(ts|tsx)$/;

// name -> what a local redeclaration would break
const SHARED_RULES: Record<string, string> = {
  DEDICATED_ROUTES: "music/photo libraries would stop redirecting to their dedicated screens",
  DEFAULT_TYPE_FILTER: "a shows library would stop opening on series only",
  AUTO_FETCH_FILL_TARGET: "the fetch-until-filled target would drift between clients",
  AUTO_FETCH_ITEM_CAP: "an unbounded auto-fetch could walk an entire library",
  IN_LIBRARY_SEARCH_LIMIT: "in-library search would return a different result set per client",
};

// `export function shouldAutoFetchMore` / `const DEDICATED_ROUTES` etc.
function declarationPattern(name: string): RegExp {
  return new RegExp(`(?:^|\\n)\\s*(?:export\\s+)?(?:const|let|var|function|class|enum)\\s+${name}\\b`);
}

function sourceFiles(dir: string): string[] {
  const out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (SKIP_DIRS.has(entry)) continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      out.push(...sourceFiles(full));
    } else if (SOURCE_EXT.test(entry)) {
      out.push(full);
    }
  }
  return out;
}

describe("shared library rules are not redeclared per app", () => {
  const files = sourceFiles(APPS_DIR);

  it("finds app sources to scan (guards against a silently empty sweep)", () => {
    expect(files.length).toBeGreaterThan(100);
  });

  it.each(Object.entries(SHARED_RULES))("%s is declared only in @evey/api", (name, consequence) => {
    const pattern = declarationPattern(name);
    const offenders = files
      .filter((f) => pattern.test(readFileSync(f, "utf8")))
      .map((f) => relative(APPS_DIR, f));

    expect(
      offenders,
      `${name} is redeclared locally - import it from @evey/api instead. ` +
        `If the copies drift, ${consequence}. Offending files: ${offenders.join(", ")}`,
    ).toEqual([]);
  });
});
