import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Card } from "./Card";

describe("Card", () => {
  it("renders children", () => {
    render(<Card>Hello card</Card>);
    expect(screen.getByText("Hello card")).toBeInTheDocument();
  });

  it("applies className", () => {
    const { container } = render(<Card className="my-custom-class">content</Card>);
    expect(container.firstChild).toHaveClass("my-custom-class");
  });

  it("passes extra props to div", () => {
    render(<Card data-testid="my-card">content</Card>);
    expect(screen.getByTestId("my-card")).toBeInTheDocument();
  });
});
