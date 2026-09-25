import { describe, expect, it } from "vitest";
import { changesUrl, latestLabel, type UpdateStatus } from "./useUpdateStatus";

const base: UpdateStatus = {
  state: "update_available",
  currentVersion: "beta-0.0.1-full",
  currentCommit: "cbcd835ec767d3cbe4084bcce53e45aad1652eda",
  source: "https://github.com/Evey-project/evey",
};

describe("update status labels", () => {
  it("names a branch build by its commit and links the commit range", () => {
    const st = { ...base, latestVersion: "beta-0.0.1-full", latestCommit: "e77dc0a31ffffffff" };
    expect(latestLabel(st)).toBe("e77dc0a");
    expect(changesUrl(st)).toBe("https://github.com/Evey-project/evey/compare/cbcd835ec767d3cbe4084bcce53e45aad1652eda...e77dc0a31ffffffff");
  });

  it("names a release by its tag and links the release notes", () => {
    const st = { ...base, currentVersion: "1.2.3-full", latestVersion: "1.2.4-full" };
    expect(latestLabel(st)).toBe("1.2.4-full");
    expect(changesUrl(st)).toBe("https://github.com/Evey-project/evey/releases");
  });

  it("never links anything but an https source", () => {
    expect(changesUrl({ ...base, latestVersion: "1.2.4", source: "javascript:alert(1)" })).toBeNull();
    expect(changesUrl({ ...base, source: undefined })).toBeNull();
  });
});
