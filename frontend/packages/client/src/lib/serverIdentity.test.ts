import { describe, it, expect } from "vitest";
import { parseEveyHealth } from "./serverIdentity";

describe("parseEveyHealth", () => {
  it("accepts a real Evey healthz payload", () => {
    expect(
      parseEveyHealth({ status: "ok", db: "ok", meilisearch: "ok", version: "beta-0.0.1", uptime_seconds: 4406 }),
    ).toEqual({ status: "ok", version: "beta-0.0.1" });
  });

  it("accepts a degraded but identified server (the app must still connect)", () => {
    expect(parseEveyHealth({ status: "degraded", version: "1.2.3" })).toEqual({ status: "degraded", version: "1.2.3" });
  });

  it("rejects what a captive portal / DNS hijack / router answers", () => {
    expect(parseEveyHealth("<!doctype html><html>Login to WiFi</html>")).toBeNull();
    expect(parseEveyHealth({ message: "welcome" })).toBeNull();
    expect(parseEveyHealth({ status: "ok" })).toBeNull(); // no version: not ours
    expect(parseEveyHealth({ version: "1.0" })).toBeNull();
    expect(parseEveyHealth(null)).toBeNull();
    expect(parseEveyHealth([])).toBeNull();
  });

  it("carries the advertised server name, trimmed, and drops a blank one", () => {
    expect(parseEveyHealth({ status: "ok", version: "1", name: " Salon " })).toEqual({ status: "ok", version: "1", name: "Salon" });
    expect(parseEveyHealth({ status: "ok", version: "1", name: "  " })).toEqual({ status: "ok", version: "1" });
    expect(parseEveyHealth({ status: "ok", version: "1", name: 42 })).toEqual({ status: "ok", version: "1" });
  });
});
