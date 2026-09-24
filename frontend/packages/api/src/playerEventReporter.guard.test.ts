import { describe, it, expect } from "vitest";
import { readFileSync } from "node:fs";
import { join } from "node:path";

// A fatal native playback error must LEAVE A TRACE on the server.
//
// 2026-08-09: a tester reported "touching the screen breaks the video" on the
// mobile app and sent a photo of the error screen. Diagnosing it server-side
// was impossible: his title DIRECT-PLAYS, so every byte the server served was
// a 200 from ServeFile, and the only client trace was a console.warn on a
// phone nobody can read. The web player has beaconed this since the 2026-07-15
// incident (see createPlayerEventReporter); mobile set its error state and
// dropped the message on the floor.
//
// This guard is on the DATA, not on a name: it asserts the branch that decides
// "playback is dead" also emits the beacon that makes it visible. The check
// scans source because the failure it prevents is a future edit quietly
// deleting the call - which no unit test of the reporter itself would catch.
const MOBILE_PLAYER = join(
  __dirname, "..", "..", "..", "apps", "mobile", "app", "(auth)", "player", "[id].tsx",
);

describe("a fatal native playback error is beaconed, not swallowed", () => {
  const src = readFileSync(MOBILE_PLAYER, "utf8");

  it("reads the mobile player screen", () => {
    // Without this the assertions below pass vacuously if the file moves.
    expect(src.length).toBeGreaterThan(10_000);
    expect(src).toContain("statusChange");
  });

  it("reports the error before latching the error screen", () => {
    // The whole `status === "error"` branch, up to its closing brace.
    const branch = src.match(/if \(ev\.status === "error"\) \{[\s\S]*?\n {6}\}/);
    expect(branch, "the fatal-error branch must still exist").not.toBeNull();
    const body = branch![0];

    expect(body, "must emit the beacon").toContain("reportPlayerEvent(");
    expect(body, "must be flagged fatal").toContain("fatal: true");
    // The native message is the ONLY thing that identifies the failure - a
    // beacon without it says "something broke" and diagnoses nothing.
    expect(body).toContain("ev.error?.message");
    // Position matters: "dies at 0:00" and "dies at 21 min" are different bugs.
    expect(body).toContain("posSec");

    // Ordering: the report must precede the state flip. Once setNativeError
    // renders ErrorScreen the player subtree unmounts, and a beacon fired
    // after that races the teardown.
    expect(body.indexOf("reportPlayerEvent(")).toBeLessThan(body.indexOf("setNativeError("));
  });

  it("builds the reporter from the shared package, not a local copy", () => {
    expect(src).toContain("createPlayerEventReporter");
    // A second implementation would drift out of the dedupe/budget contract
    // that keeps a looping failure from flooding the admin log.
    expect(src).not.toMatch(/function createPlayerEventReporter/);
  });
});

// The browser television had no beacon at all until 2026-09-24, and on its
// native path (direct play, the set's own HLS) an element error left a black
// screen instead of the error screen.
const TV_WEB_PLAYER = join(__dirname, "..", "..", "..", "apps", "tv-web", "src", "screens", "Player.tsx");

describe("the browser television beacons its playback errors", () => {
  const src = readFileSync(TV_WEB_PLAYER, "utf8");

  it("reads the tv-web player screen", () => {
    expect(src.length).toBeGreaterThan(10_000);
  });

  it("reports hls.js and element errors through the shared reporter", () => {
    expect(src).toContain("createPlayerEventReporter(mediaId)");
    expect(src).not.toMatch(/function createPlayerEventReporter/);
    expect(src).toContain('"hls_fatal"');
    expect(src).toContain('"video_element_error"');
  });

  it("listens for the element error on the native paths", () => {
    expect(src.match(/addEventListener\("error", (nativeError|elementError)\)/g)?.length ?? 0).toBeGreaterThanOrEqual(3);
  });
});
