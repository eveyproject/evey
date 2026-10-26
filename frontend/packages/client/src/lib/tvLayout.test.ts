import { describe, expect, it } from "vitest";
import { gridColumns } from "./tvLayout";

describe("gridColumns", () => {
  it("fills the width the screen has: six 165dp cards on a 1200dp television, five on a 960dp one", () => {
    expect(gridColumns(1200 - 48, 165, 12)).toBe(6);
    expect(gridColumns(960 - 48, 165, 12)).toBe(5);
  });
  it("never answers less than one column", () => {
    expect(gridColumns(100, 165, 12)).toBe(1);
  });
});
