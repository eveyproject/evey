import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Spinner } from "./Spinner";

describe("Spinner", () => {
  it("renders with aria-label", () => {
    render(<Spinner />);
    expect(screen.getByLabelText("Loading")).toBeInTheDocument();
  });

  it("applies sm size classes", () => {
    render(<Spinner size="sm" />);
    const el = screen.getByLabelText("Loading");
    expect(el).toHaveClass("w-4", "h-4");
  });

  it("applies md size classes (default)", () => {
    render(<Spinner />);
    const el = screen.getByLabelText("Loading");
    expect(el).toHaveClass("w-8", "h-8");
  });

  it("applies lg size classes", () => {
    render(<Spinner size="lg" />);
    const el = screen.getByLabelText("Loading");
    expect(el).toHaveClass("w-12", "h-12");
  });

  it("forwards extra className", () => {
    render(<Spinner className="extra" />);
    expect(screen.getByLabelText("Loading")).toHaveClass("extra");
  });
});
