import { existsSync, readFileSync, readdirSync, statSync } from "node:fs";
import { dirname, join, resolve } from "node:path";
import { describe, it, expect } from "vitest";
import { parse } from "yaml";

// Every t("...") in the app must resolve against the server's catalogue.
//
// Nothing enforced this, and the failure is silent by construction: t() falls
// back to its second argument, so an English-speaking developer sees the right
// words and ships a string that is untranslated in the five other languages.
// The user hit it on a real television - "Failed to load stream" in a French
// UI - and the audit that followed found three more, three of them added the
// same night by a developer who "checked" with a grep that matched a key of
// the same name in a different block.
//
// A grep cannot answer this question. The catalogue is nested and i18next
// resolves plurals through suffixes, so the only honest check parses both
// sides. That is what this does.
//
// Scope note: this started life guarding the TV app, where the bug surfaced.
// It now covers web and mobile too - widening it turned up two mobile keys
// that had been rendering their English fallback on a French screen since the
// day they were written (2026-08-04).

// Walk up to the repo root rather than counting "../" from import.meta.url:
// under vite that URL carries a /@fs prefix, and a hardcoded depth breaks the
// day this file moves.
// Returns null rather than throwing: the CI container mounts only `frontend`
// at /app, so the catalogue is genuinely out of reach there. A guard that
// cannot see both sides must SKIP visibly - a thrown error turns the whole
// suite red, and a silent pass would be worse still.
function repoRoot(): string | null {
  let dir = resolve(process.cwd());
  for (let i = 0; i < 8; i++) {
    if (existsSync(join(dir, "i18n", "en"))) return dir;
    const parent = dirname(dir);
    if (parent === dir) break;
    dir = parent;
  }
  return null;
}

const REPO_ROOT = repoRoot();
// This runs on the host through `make tv-audit` and `make pre-push`.
const CAN_RUN = REPO_ROOT !== null;
const CATALOGUE_DIR = join(REPO_ROOT ?? "", "i18n", "en");
const APPS = ["tv", "web", "mobile"] as const;
const APP_ROOT = join(REPO_ROOT ?? "", "frontend", "apps", "tv");

// i18next picks x_one/x_other/... when the caller passes a count, so a key
// that only exists in plural form still resolves.
const PLURAL_SUFFIXES = ["_one", "_other", "_zero", "_two", "_few", "_many"];

function loadCatalogue(): Set<string> {
  const keys = new Set<string>();
  for (const file of readdirSync(CATALOGUE_DIR)) {
    if (!file.endsWith(".yaml")) continue;
    const namespace = file.slice(0, -".yaml".length);
    const walk = (node: unknown, prefix: string) => {
      if (node === null || typeof node !== "object") {
        keys.add(prefix);
        return;
      }
      for (const [k, v] of Object.entries(node as Record<string, unknown>)) {
        walk(v, prefix ? `${prefix}.${k}` : k);
      }
    };
    walk(parse(readFileSync(join(CATALOGUE_DIR, file), "utf8")), namespace);
  }
  return keys;
}

function sourceFiles(dir: string, acc: string[] = []): string[] {
  for (const entry of readdirSync(dir)) {
    if (["node_modules", "android", "ios", ".expo", "build", "dist"].includes(entry)) continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) sourceFiles(full, acc);
    else if (/\.tsx?$/.test(entry) && !entry.includes(".test.")) acc.push(full);
  }
  return acc;
}

describe("every translation key the three apps ask for exists in the catalogue", () => {
  it.skipIf(!CAN_RUN).each(APPS)("%s renders no key that would silently fall back to English", (app) => {
    const catalogue = loadCatalogue();
    const resolves = (key: string) =>
      catalogue.has(key) || PLURAL_SUFFIXES.some((s) => catalogue.has(key + s));

    // Only literal keys: a computed one (`t(someVar)`) cannot be checked here,
    // and pretending otherwise would make this guard look stronger than it is.
    const call = /\bt\(\s*"([a-zA-Z0-9_.]+)"/g;
    const root = join(REPO_ROOT ?? "", "frontend", "apps", app);
    const missing: string[] = [];
    for (const file of sourceFiles(root)) {
      const src = readFileSync(file, "utf8");
      for (const m of src.matchAll(call)) {
        if (!resolves(m[1])) missing.push(`${m[1]}  (${file.slice(root.length + 1)})`);
      }
    }
    expect([...new Set(missing)].sort()).toEqual([]);
  });

  // Guards the guard: a regex that stops matching, or a catalogue that fails to
  // load, would make the assertion above pass on an empty set forever.
  it.skipIf(!CAN_RUN)("actually reads both sides", () => {
    const catalogue = loadCatalogue();
    expect(catalogue.size).toBeGreaterThan(500);
    expect(catalogue.has("player.failedToLoadStream")).toBe(true);
    expect(sourceFiles(APP_ROOT).length).toBeGreaterThan(50);
  });
});
