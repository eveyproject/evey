import { describe, it, expect } from "vitest";
import { buildSetupStatusUrl, parseServerName } from "./serverBrand";

describe("buildSetupStatusUrl", () => {
  it("targets the playing instance's public setup status", () => {
    expect(buildSetupStatusUrl("https://evey.home/api/v1/media/abc/stream/master.m3u8?token=t&abr=floor"))
      .toBe("https://evey.home/api/v1/setup/status");
  });

  it("refuses foreign or unparsable URLs", () => {
    expect(buildSetupStatusUrl("https://cdn.example/video.mp4")).toBeNull();
    expect(buildSetupStatusUrl("not a url")).toBeNull();
  });
});

describe("parseServerName", () => {
  it("keeps an admin-chosen name, trimmed", () => {
    expect(parseServerName({ setupComplete: true, serverName: " Salon " })).toBe("Salon");
  });

  it("drops the default (the logo already says Evey) and junk payloads", () => {
    expect(parseServerName({ serverName: "Evey" })).toBe("");
    expect(parseServerName({ serverName: "" })).toBe("");
    expect(parseServerName({})).toBe("");
    expect(parseServerName("<html>")).toBe("");
    expect(parseServerName(null)).toBe("");
  });
});
