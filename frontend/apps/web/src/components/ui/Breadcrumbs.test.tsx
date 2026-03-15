import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";
import { Breadcrumbs } from "./Breadcrumbs";

vi.mock("@tanstack/react-router", () => ({
  Link: ({ children, to }: { children: React.ReactNode; to: string }) => (
    <a href={to}>{children}</a>
  ),
}));

describe("Breadcrumbs", () => {
  it("renders all items", () => {
    render(
      <Breadcrumbs items={[{ label: "Home", to: "/home" }, { label: "Library", to: "/library" }, { label: "Movie" }]} />,
    );
    expect(screen.getByText("Home")).toBeInTheDocument();
    expect(screen.getByText("Library")).toBeInTheDocument();
    expect(screen.getByText("Movie")).toBeInTheDocument();
  });

  it("renders links for items with `to`", () => {
    render(
      <Breadcrumbs items={[{ label: "Home", to: "/home" }, { label: "Current" }]} />,
    );
    const link = screen.getByRole("link", { name: "Home" });
    expect(link).toHaveAttribute("href", "/home");
  });

  it("renders plain text for items without `to`", () => {
    render(
      <Breadcrumbs items={[{ label: "Home", to: "/home" }, { label: "Current" }]} />,
    );
    expect(screen.queryByRole("link", { name: "Current" })).not.toBeInTheDocument();
    expect(screen.getByText("Current")).toBeInTheDocument();
  });

  it("renders separators between items", () => {
    render(
      <Breadcrumbs items={[{ label: "A" }, { label: "B" }, { label: "C" }]} />,
    );
    const separators = screen.getAllByText("›");
    expect(separators).toHaveLength(2);
  });

  it("renders single item with no separator", () => {
    render(<Breadcrumbs items={[{ label: "Only" }]} />);
    expect(screen.getByText("Only")).toBeInTheDocument();
    expect(screen.queryByText("›")).not.toBeInTheDocument();
  });

  it("has breadcrumb nav role", () => {
    render(<Breadcrumbs items={[{ label: "Home" }]} />);
    expect(screen.getByRole("navigation", { name: /breadcrumb/i })).toBeInTheDocument();
  });

  it("applies custom className", () => {
    const { container } = render(<Breadcrumbs items={[{ label: "A" }]} className="custom-class" />);
    expect(container.firstChild).toHaveClass("custom-class");
  });
});
