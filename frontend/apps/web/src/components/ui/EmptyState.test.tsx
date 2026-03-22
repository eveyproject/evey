import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Search } from "lucide-react";
import { EmptyState } from "./EmptyState";

describe("EmptyState", () => {
  it("renders title", () => {
    render(<EmptyState title="Nothing here" />);
    expect(screen.getByText("Nothing here")).toBeInTheDocument();
  });

  it("renders description when provided", () => {
    render(<EmptyState title="Empty" description="Try searching again" />);
    expect(screen.getByText("Try searching again")).toBeInTheDocument();
  });

  it("does not render description when omitted", () => {
    const { container } = render(<EmptyState title="Empty" />);
    const paragraphs = container.querySelectorAll("p");
    expect(paragraphs).toHaveLength(1);
  });

  it("renders icon when provided", () => {
    const { container } = render(<EmptyState title="No results" icon={<Search size={48} />} />);
    expect(container.querySelector("svg")).toBeInTheDocument();
  });

  it("does not render icon when omitted", () => {
    const { container } = render(<EmptyState title="No results" />);
    expect(container.querySelector("svg")).not.toBeInTheDocument();
  });

  it("renders action button and fires onClick", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(
      <EmptyState title="Empty" action={{ label: "Retry", onClick }} />,
    );
    const btn = screen.getByRole("button", { name: "Retry" });
    expect(btn).toBeInTheDocument();
    await user.click(btn);
    expect(onClick).toHaveBeenCalledOnce();
  });
});
