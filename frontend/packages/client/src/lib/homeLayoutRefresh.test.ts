import { describe, it, expect } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { emitSettingsChanged, followSettingsChanges, refreshSettings, refreshSettingsOnForeground } from "./homeLayoutRefresh";

const FEED = ["/api/v1/feeds/config", { profileId: "p1" }];
const PROFILES = ["/api/v1/users/u1/profiles"];
const LIBRARIES = ["/api/v1/libraries"];

function seeded(): QueryClient {
  const qc = new QueryClient();
  qc.setQueryData(FEED, { configs: [] });
  qc.setQueryData(PROFILES, { profiles: [] });
  qc.setQueryData(LIBRARIES, { libraries: [] });
  return qc;
}

const stale = (qc: QueryClient, key: unknown[]) => qc.getQueryState(key)?.isInvalidated;

describe("refreshSettings", () => {
  it("reloads only the profile for a profile change", () => {
    const qc = seeded();
    refreshSettings(qc, "profile");
    expect(stale(qc, PROFILES)).toBe(true);
    expect(stale(qc, FEED)).toBe(false);
    expect(stale(qc, LIBRARIES)).toBe(false);
  });

  it("reloads only the home layout for a layout change", () => {
    const qc = seeded();
    refreshSettings(qc, "homeLayout");
    expect(stale(qc, FEED)).toBe(true);
    expect(stale(qc, PROFILES)).toBe(false);
  });

  // A scope this build does not know yet must not be dropped on the floor.
  it("reloads both for an unknown or missing scope", () => {
    for (const scope of [undefined, "somethingNewer"]) {
      const qc = seeded();
      refreshSettings(qc, scope);
      expect(stale(qc, PROFILES)).toBe(true);
      expect(stale(qc, FEED)).toBe(true);
      expect(stale(qc, LIBRARIES)).toBe(false);
    }
  });
});

describe("refreshSettingsOnForeground", () => {
  // Settings changed on another device must show once the viewer comes back
  // to this one; the other queries are untouched.
  it("reloads the profile and the home layout on foreground", () => {
    const qc = seeded();
    let fire: () => void = () => {};
    const unsubscribe = refreshSettingsOnForeground(qc, (listener) => {
      fire = listener;
      return () => { fire = () => {}; };
    });

    fire();
    expect(stale(qc, FEED)).toBe(true);
    expect(stale(qc, PROFILES)).toBe(true);
    expect(stale(qc, LIBRARIES)).toBe(false);
    unsubscribe();
  });
});

describe("followSettingsChanges", () => {
  // The push from another device and the return to the foreground both reload;
  // once unsubscribed, neither does.
  it("reloads on a push with its scope, and stops when unsubscribed", () => {
    const qc = seeded();
    const stop = followSettingsChanges(qc, () => () => {});
    emitSettingsChanged("homeLayout");
    expect(stale(qc, FEED)).toBe(true);
    expect(stale(qc, PROFILES)).toBe(false);

    stop();
    emitSettingsChanged("profile");
    expect(stale(qc, PROFILES)).toBe(false);
  });
});
