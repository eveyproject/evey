import { describe, it, expect } from "vitest";
import { interpolateFallback } from "./interpolateFallback";

describe("interpolateFallback", () => {
  it("leaves an already-interpolated string unchanged", () => {
    expect(interpolateFallback("3 server(s) found", { count: 3 })).toBe(
      "3 server(s) found",
    );
  });

  it("replaces {{count}} with the given value", () => {
    expect(interpolateFallback("{{count}} server(s) found", { count: 3 })).toBe(
      "3 server(s) found",
    );
  });

  it("leaves an absent key intact", () => {
    expect(interpolateFallback("{{name}} will be removed.", {})).toBe(
      "{{name}} will be removed.",
    );
  });

  it("replaces multiple variables", () => {
    expect(
      interpolateFallback("Use discovered server {{url}} ({{count}})", {
        url: "https://evey.test",
        count: 2,
      }),
    ).toBe("Use discovered server https://evey.test (2)");
  });
});
