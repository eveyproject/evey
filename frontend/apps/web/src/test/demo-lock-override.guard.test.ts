import { readdirSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative } from "node:path";
import { describe, it, expect } from "vitest";

// Ratchet: a `disabled` written AFTER `{...lock}` on the same element wins over
// the demo lock - JSX keeps the last prop - so the control stays clickable on
// the demo and the server's refusal is the only thing left. Sixteen controls
// had that shape (ratings, collection edits, bulk actions, a plugin toggle,
// mark-read...), found 2026-10-01. A disabled that follows the lock must carry
// it: `disabled={!!lock.disabled || busy}`.

const srcDir = join(dirname(fileURLToPath(import.meta.url)), "..");

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return walk(full);
    return full.endsWith(".tsx") && !full.endsWith(".test.tsx") ? [full] : [];
  });
}

// The attributes of the element the spread sits in: from the spread up to the
// first `>` outside any brace (arrow functions and template literals live
// inside braces, so their `>` does not end the tag).
function tagAfter(src: string, from: number): string {
  let depth = 0;
  for (let i = from; i < src.length; i++) {
    const ch = src[i];
    if (ch === "{") depth++;
    else if (ch === "}") depth--;
    else if (ch === ">" && depth === 0) return src.slice(from, i);
  }
  return src.slice(from);
}

describe("the demo lock is never overridden", () => {
  it("every disabled that follows {...lock} includes lock.disabled", () => {
    const offenders: string[] = [];
    for (const file of walk(srcDir)) {
      const src = readFileSync(file, "utf8");
      for (const m of src.matchAll(/\{\.\.\.lock\}/g)) {
        const tag = tagAfter(src, (m.index ?? 0) + m[0].length);
        const disabled = /\sdisabled=\{/.exec(tag);
        if (disabled && !tag.slice(disabled.index).split("\n")[0].includes("lock.disabled")) {
          const line = src.slice(0, m.index).split("\n").length;
          offenders.push(`${relative(srcDir, file)}:${line}`);
        }
      }
    }
    expect(offenders).toEqual([]);
  });
});
