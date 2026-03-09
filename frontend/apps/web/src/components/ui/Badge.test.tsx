import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Badge } from "./Badge";

describe("Badge", () => {
  it("renders text content", () => {
    render(<Badge>New</Badge>);
    expect(screen.getByText("New")).toBeInTheDocument();
  });

  it("applies default variant", () => {
    render(<Badge>Tag</Badge>);
    expect(screen.getByText("Tag")).toHaveClass("bg-[var(--surface-raised)]");
  });

  it.each([
    ["success", "bg-[var(--badge-success-bg,#065f46)]"],
    ["warning", "bg-[var(--badge-warning-bg,#713f12)]"],
    ["error", "bg-[var(--badge-error-bg,#7f1d1d)]"],
    ["info", "bg-[var(--badge-info-bg,#1e3a5f)]"],
  ] as const)("applies %s variant class", (variant, expectedClass) => {
    render(<Badge variant={variant}>V</Badge>);
    expect(screen.getByText("V")).toHaveClass(expectedClass);
  });

  it("forwards extra className", () => {
    render(<Badge className="extra">X</Badge>);
    expect(screen.getByText("X")).toHaveClass("extra");
  });
});
