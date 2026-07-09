import { describe, it, expect } from "vitest";
import { hitTestGalaxy } from "../src/domain/services/hitTest";

describe("hitTestGalaxy", () => {
  const nodes = [
    { x: -0.5, y: 0, radius: 0.2 },
    { x: 0.5, y: 0, radius: 0.2 },
    { x: 0, y: 0.5, radius: 0.1 },
  ];

  it("finds the node under the pointer", () => {
    expect(hitTestGalaxy(nodes, { x: -0.5, y: 0 })).toBe(0);
    expect(hitTestGalaxy(nodes, { x: 0.5, y: 0.05 })).toBe(1);
  });

  it("returns null when the pointer is over nothing", () => {
    expect(hitTestGalaxy(nodes, { x: -5, y: -5 })).toBeNull();
  });

  it("prefers the smaller (frontmost) node when circles overlap", () => {
    const overlapping = [
      { x: 0, y: 0, radius: 0.5 },
      { x: 0, y: 0, radius: 0.1 },
    ];
    expect(hitTestGalaxy(overlapping, { x: 0, y: 0 })).toBe(1);
  });

  it("returns null for an empty node list", () => {
    expect(hitTestGalaxy([], { x: 0, y: 0 })).toBeNull();
  });

  it("is inclusive of the exact edge of a circle", () => {
    const node = [{ x: 0, y: 0, radius: 1 }];
    expect(hitTestGalaxy(node, { x: 1, y: 0 })).toBe(0);
  });
});
