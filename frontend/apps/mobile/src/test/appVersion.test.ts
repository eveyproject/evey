import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// beta.36 shipped announcing itself as beta.35.
//
// The APK's own version comes from android/app/build.gradle, but everything the
// user and the server SEE comes from app.json: settings.tsx renders
// Constants.expoConfig?.version, and clientIdentity.ts puts it in the user
// agent, so the admin device list records whatever app.json says. The bump
// commit touched build.gradle only, and nothing anywhere compared the two -
// the build was green, the APK installed, and every tester running beta.36
// reported beta.35 to the server.
//
// Nothing at runtime can catch this: both files are build inputs, and the one
// that lies is the one the tests never read.
const MOBILE_ROOT = join(__dirname, "..", "..");

function appJsonVersion(): string {
  const raw = readFileSync(join(MOBILE_ROOT, "app.json"), "utf8");
  const parsed = JSON.parse(raw) as { expo?: { version?: unknown } };
  const version = parsed.expo?.version;
  if (typeof version !== "string") {
    throw new Error("app.json has no expo.version string");
  }
  return version;
}

function gradleVersion(): { name: string; code: number } {
  const raw = readFileSync(join(MOBILE_ROOT, "android", "app", "build.gradle"), "utf8");
  const name = raw.match(/^\s*versionName\s+"([^"]+)"/m);
  const code = raw.match(/^\s*versionCode\s+(\d+)/m);
  if (!name || !code) {
    // Without this the suite would pass vacuously the day the block moves or
    // the syntax changes - the exact way a static guard stops guarding.
    throw new Error("build.gradle has no versionName/versionCode in defaultConfig");
  }
  return { name: name[1], code: Number(code[1]) };
}

describe("mobile app version", () => {
  it("reads a plausible version from both build inputs", () => {
    expect(appJsonVersion()).toMatch(/^\d+\.\d+\.\d+/);
    expect(gradleVersion().code).toBeGreaterThan(0);
  });

  it("announces the same version to the user as the one it ships under", () => {
    const fromApp = appJsonVersion();
    const fromGradle = gradleVersion().name;
    expect(
      fromApp,
      `app.json says "${fromApp}" but the APK ships as "${fromGradle}" - the settings screen and the user agent sent to the server would both report the wrong build. Bump BOTH files.`,
    ).toBe(fromGradle);
  });
});
