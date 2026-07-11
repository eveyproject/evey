import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { Avatar } from "./Avatar";

describe("Avatar", () => {
  it("shows the initial when there is no image", () => {
    render(<Avatar initial="R" />);
    expect(screen.getByText("R")).toBeInTheDocument();
  });

  it("renders the image and drops the initial when imageUrl is set", () => {
    const { container } = render(<Avatar initial="R" imageUrl="/api/v1/profiles/p1/avatar" />);
    const img = container.querySelector("img");
    expect(img).toHaveAttribute("src", "/api/v1/profiles/p1/avatar");
    expect(screen.queryByText("R")).not.toBeInTheDocument();
  });

  it("falls back to the initial when imageUrl is null", () => {
    render(<Avatar initial="A" imageUrl={null} />);
    expect(screen.getByText("A")).toBeInTheDocument();
  });

  it("sizes the circle from the size prop", () => {
    const { container } = render(<Avatar initial="R" size={48} />);
    const span = container.firstChild as HTMLElement;
    expect(span.style.width).toBe("48px");
    expect(span.style.height).toBe("48px");
  });
});
