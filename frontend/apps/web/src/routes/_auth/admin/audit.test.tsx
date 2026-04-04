import { describe, it, expect } from "vitest";

describe("Redirected page", () => {
  it("page module exports a route", async () => {
    const mod = await import(`./audit.lazy`);
    expect(mod.Route).toBeDefined();
  });
});
