import { describe, it, expect } from "vitest";
import { readdirSync, readFileSync, statSync } from "node:fs";
import { join, relative } from "node:path";

// Guard against the failure that killed offline downloads in beta.22.
//
// expo-file-system 55 still EXPORTS the legacy file helpers from the package
// root, but every one of them is a stub whose body is `throw new Error(...)`.
// The call therefore fails on the first invocation at runtime, on device only:
// the type-checker is happy (the declarations are real), the linter is happy,
// and every unit test is happy because the suites mock "expo-file-system" with
// a module shape that does not exist. downloads.ts called getInfoAsync on its
// very first line of work, so the whole feature died before it ever reached the
// network - the server never even saw the request.
//
// A runtime test cannot catch this (the module is always mocked), so the guard
// has to be static: these names must come from "expo-file-system/legacy".
const LEGACY_ONLY = [
  "getInfoAsync",
  "readAsStringAsync",
  "writeAsStringAsync",
  "deleteAsync",
  "makeDirectoryAsync",
  "readDirectoryAsync",
  "moveAsync",
  "copyAsync",
  "downloadAsync",
  "uploadAsync",
  "createDownloadResumable",
  "getContentUriAsync",
  "getFreeDiskStorageAsync",
  "getTotalDiskCapacityAsync",
  "documentDirectory",
  "cacheDirectory",
  "bundleDirectory",
];

const SRC_ROOTS = [join(__dirname, ".."), join(__dirname, "..", "..", "app")];

function sourceFiles(dir: string): string[] {
  let out: string[] = [];
  for (const entry of readdirSync(dir)) {
    if (entry === "node_modules") continue;
    const full = join(dir, entry);
    if (statSync(full).isDirectory()) {
      out = out.concat(sourceFiles(full));
    } else if (/\.tsx?$/.test(entry)) {
      out.push(full);
    }
  }
  return out;
}

// Matches `import ... from "expo-file-system"` but not `.../legacy`. The
// clause is [^;]* rather than [\s\S]*? so it cannot run past the end of a
// statement and swallow unrelated prose - this file's own comments mention
// every name in LEGACY_ONLY, and a greedier pattern reported itself.
const ROOT_IMPORT = /import\s+([^;]*?)\s+from\s+["']expo-file-system["']/g;

describe("expo-file-system imports", () => {
  const files = SRC_ROOTS.flatMap(sourceFiles).filter((f) => f !== __filename);

  it("scans a non-trivial number of sources", () => {
    // Without this the suite would pass vacuously if the roots ever move.
    expect(files.length).toBeGreaterThan(50);
  });

  it("never imports a runtime-throwing legacy symbol from the package root", () => {
    const offenders: string[] = [];
    for (const file of files) {
      const src = readFileSync(file, "utf8");
      for (const match of src.matchAll(ROOT_IMPORT)) {
        const clause = match[1];
        // `import * as FileSystem from "expo-file-system"` is the dangerous
        // shape: every legacy name is then reachable off the namespace, and
        // nothing here can tell which one gets called.
        if (/^\s*\*\s+as\s+/.test(clause)) {
          offenders.push(`${relative(process.cwd(), file)}: namespace import of the package root`);
          continue;
        }
        for (const name of LEGACY_ONLY) {
          if (new RegExp(`\\b${name}\\b`).test(clause)) {
            offenders.push(`${relative(process.cwd(), file)}: ${name}`);
          }
        }
      }
    }
    expect(
      offenders,
      `these must import from "expo-file-system/legacy" - the package root exports stubs that throw at runtime:\n${offenders.join("\n")}`,
    ).toEqual([]);
  });
});
