import { readdirSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative } from "node:path";
import { describe, it, expect } from "vitest";

// Ratchet: "the request failed" belongs to FetchErrorState, once.
//
// Eleven screens had grown their own copy of it - the same EmptyState, the same
// classifyError ternary, the same two keys - and a copy is where the offline/
// server distinction silently rots back into "your library is empty" (audited
// 2026-08-04, re-found 2026-08-09). A screen that classifies an error AND draws
// its own error EmptyState is re-growing that copy.

const srcDir = join(dirname(fileURLToPath(import.meta.url)), "..");

// The component itself, and the two error surfaces that are not fetch failures:
// the React boundary and the router's own error element.
const ALLOWED = ["components/ui/FetchErrorState.tsx", "components/ui/ErrorBoundary.tsx"];

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return walk(full);
    return full.endsWith(".tsx") && !full.endsWith(".test.tsx") ? [full] : [];
  });
}

describe("no screen re-implements FetchErrorState", () => {
  it("only FetchErrorState pairs classifyError with an error EmptyState", () => {
    const offenders = walk(srcDir).filter((file) => {
      const rel = relative(srcDir, file);
      if (ALLOWED.includes(rel)) return false;
      const src = readFileSync(file, "utf8");
      return src.includes("classifyError") && src.includes("common.errorBoundary.title");
    });

    expect(offenders.map((f) => relative(srcDir, f))).toEqual([]);
  });
});
