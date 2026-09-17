import { readdirSync, readFileSync, statSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { dirname, join, relative } from "node:path";
import { describe, it, expect } from "vitest";

// Ratchet: crypto.randomUUID is a secure-context API - undefined on the
// documented quick-start URL (http://<nas-ip>:9091). A bare call at module
// scope in GlobalAudioPlayer blanked the whole app for a Synology install
// (2026-09-17); dev on localhost and prod behind https never saw it. Every
// use goes through lib/randomId, which falls back to getRandomValues.

const srcDir = join(dirname(fileURLToPath(import.meta.url)), "..");
const ALLOWED = ["lib/randomId.ts"];

function walk(dir: string): string[] {
  return readdirSync(dir).flatMap((entry) => {
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) return walk(full);
    return /\.tsx?$/.test(full) && !/\.test\.tsx?$/.test(full) ? [full] : [];
  });
}

describe("secure-context APIs are wrapped", () => {
  it("only lib/randomId calls crypto.randomUUID()", () => {
    const offenders = walk(srcDir).filter((file) => {
      const rel = relative(srcDir, file);
      if (ALLOWED.includes(rel)) return false;
      return /crypto\.randomUUID\s*\(/.test(readFileSync(file, "utf8"));
    });
    expect(offenders.map((f) => relative(srcDir, f))).toEqual([]);
  });
});
