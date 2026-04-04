import { describe, it, expect } from "vitest";

describe("Profile index redirect", () => {
  it("page module exports a route", async () => {
    const mod = await import("./index.lazy");
    expect(mod.Route).toBeDefined();
  });
});
