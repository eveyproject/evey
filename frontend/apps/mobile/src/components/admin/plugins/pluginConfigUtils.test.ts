import { describe, it, expect } from "vitest";
import { parseEscalationEntries } from "./pluginConfigUtils";

describe("parseEscalationEntries", () => {
  it("extracts the comma-separated entries after the marker", () => {
    const msg = "rpc error: code = FailedPrecondition desc = new entries requested: permission:network, capability:foo";
    expect(parseEscalationEntries(msg)).toEqual(["permission:network", "capability:foo"]);
  });

  it("returns an empty array when the marker is absent", () => {
    expect(parseEscalationEntries("some unrelated error message")).toEqual([]);
  });

  it("trims whitespace and drops empty segments", () => {
    const msg = "new entries requested:  permission:network ,  , capability:foo ";
    expect(parseEscalationEntries(msg)).toEqual(["permission:network", "capability:foo"]);
  });

  it("handles a single entry", () => {
    expect(parseEscalationEntries("new entries requested: permission:network")).toEqual(["permission:network"]);
  });
});
