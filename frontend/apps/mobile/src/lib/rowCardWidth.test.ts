import { describe, expect, it, vi } from "vitest";

vi.mock("react-native", () => ({ Dimensions: { get: () => ({ width: 390, height: 844 }) } }));

import { ROW_CARD_MAX_WIDTH, rowCardWidth } from "./rowCardWidth";

describe("rowCardWidth", () => {
  it("keeps a phone's proportional card", () => {
    expect(rowCardWidth(390)).toBeCloseTo(136.5);
    expect(rowCardWidth(430)).toBeCloseTo(150.5);
  });

  it("stops growing on a tablet", () => {
    expect(rowCardWidth(800)).toBe(ROW_CARD_MAX_WIDTH);
    expect(rowCardWidth(1280)).toBe(ROW_CARD_MAX_WIDTH);
  });
});
