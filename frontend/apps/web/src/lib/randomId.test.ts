import { afterEach, describe, expect, it, vi } from "vitest";
import { randomUUID } from "./randomId";

const V4 = /^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$/;

describe("randomUUID", () => {
  afterEach(() => {
    vi.unstubAllGlobals();
  });

  it("uses crypto.randomUUID when the browser exposes it", () => {
    vi.stubGlobal("crypto", { randomUUID: () => "11111111-2222-4333-8444-555555555555" });
    expect(randomUUID()).toBe("11111111-2222-4333-8444-555555555555");
  });

  it("still yields a v4 UUID on plain http, where randomUUID is undefined", () => {
    // What a browser exposes on http://<ip>: getRandomValues only.
    vi.stubGlobal("crypto", {
      getRandomValues: (b: Uint8Array) => {
        for (let i = 0; i < b.length; i++) b[i] = (i * 37 + 11) & 0xff;
        return b;
      },
    });
    const id = randomUUID();
    expect(id).toMatch(V4);
  });

  it("does not throw with no crypto at all", () => {
    vi.stubGlobal("crypto", undefined);
    expect(randomUUID()).toMatch(V4);
    expect(randomUUID()).not.toBe(randomUUID());
  });
});
