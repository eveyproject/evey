import { describe, it, expect, beforeEach } from "vitest";
import { useCssVar } from "./useCssVar";

beforeEach(() => {
  // Reset inline custom props on <html>
  document.documentElement.style.removeProperty("--test-color");
  document.documentElement.style.removeProperty("--test-blank");
});

describe("useCssVar", () => {
  it("returns the CSS variable value when set", () => {
    document.documentElement.style.setProperty("--test-color", "#ff00ff");
    expect(useCssVar("--test-color")).toBe("#ff00ff");
  });

  it("trims whitespace around the value", () => {
    document.documentElement.style.setProperty("--test-color", "  red  ");
    expect(useCssVar("--test-color")).toBe("red");
  });

  it("falls back when the variable is undefined", () => {
    expect(useCssVar("--does-not-exist")).toBe("#888");
  });

  it("falls back when the variable is empty", () => {
    document.documentElement.style.setProperty("--test-blank", "");
    expect(useCssVar("--test-blank", "#abc")).toBe("#abc");
  });

  it("uses the explicit fallback over the default", () => {
    expect(useCssVar("--missing", "tomato")).toBe("tomato");
  });
});
