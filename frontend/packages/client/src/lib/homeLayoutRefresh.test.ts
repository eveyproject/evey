import { describe, it, expect } from "vitest";
import { QueryClient } from "@tanstack/react-query";
import { refreshHomeLayoutOnForeground } from "./homeLayoutRefresh";

describe("refreshHomeLayoutOnForeground", () => {
  // The layout changed on another device must show once the viewer comes back
  // to this one: the feed config goes stale, the other queries are untouched.
  it("invalidates the feed config, and only it, on foreground", () => {
    const qc = new QueryClient();
    qc.setQueryData(["/api/v1/feeds/config", { profileId: "p1" }], { configs: [] });
    qc.setQueryData(["/api/v1/libraries"], { libraries: [] });
    let fire: () => void = () => {};
    const unsubscribe = refreshHomeLayoutOnForeground(qc, (listener) => {
      fire = listener;
      return () => { fire = () => {}; };
    });

    fire();
    expect(qc.getQueryState(["/api/v1/feeds/config", { profileId: "p1" }])?.isInvalidated).toBe(true);
    expect(qc.getQueryState(["/api/v1/libraries"])?.isInvalidated).toBe(false);
    unsubscribe();
  });
});
