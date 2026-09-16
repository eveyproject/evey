import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";
import { CollapsibleCard } from "./CollapsibleCard";

vi.mock("./InfoTooltip", () => ({
  InfoTooltip: ({ label, children }: any) => React.createElement("span", { "data-testid": "info", "aria-label": label }, children),
}));

// A collapsible section hides its body until the header is clicked or
// activated from the keyboard (Enter/Space, without scrolling the page); an
// always-open card has no button semantics at all and never hides its body.
describe("CollapsibleCard", () => {
  it("starts closed, toggles on click and reports aria-expanded", () => {
    render(<CollapsibleCard title="Section" subtitle="Details"><p>Body</p></CollapsibleCard>);
    expect(screen.queryByText("Body")).toBeNull();
    const header = screen.getByRole("button");
    expect(header.getAttribute("aria-expanded")).toBe("false");
    expect(screen.getByText("Details")).toBeInTheDocument();

    fireEvent.click(header);
    expect(screen.getByText("Body")).toBeInTheDocument();
    expect(header.getAttribute("aria-expanded")).toBe("true");
    fireEvent.click(header);
    expect(screen.queryByText("Body")).toBeNull();
  });

  it("toggles from the keyboard with Enter and Space only", () => {
    render(<CollapsibleCard title="Section"><p>Body</p></CollapsibleCard>);
    const header = screen.getByRole("button");
    expect(header.getAttribute("tabindex")).toBe("0");
    fireEvent.keyDown(header, { key: "a" });
    expect(screen.queryByText("Body")).toBeNull();
    fireEvent.keyDown(header, { key: "Enter" });
    expect(screen.getByText("Body")).toBeInTheDocument();
    const space = fireEvent.keyDown(header, { key: " " });
    expect(space).toBe(false);
    expect(screen.queryByText("Body")).toBeNull();
  });

  it("honours defaultOpen and shows the info popover next to the title", () => {
    render(<CollapsibleCard title="Section" defaultOpen info="More"><p>Body</p></CollapsibleCard>);
    expect(screen.getByText("Body")).toBeInTheDocument();
    expect(screen.getByTestId("info")).toHaveTextContent("More");
    expect(screen.getByTestId("info").getAttribute("aria-label")).toBe("Section");
  });

  it("an always-open card has no button, no chevron and keeps its body", () => {
    render(<CollapsibleCard title="Section" alwaysOpen icon={<i data-testid="icon" />}><p>Body</p></CollapsibleCard>);
    expect(screen.queryByRole("button")).toBeNull();
    expect(screen.getByText("Body")).toBeInTheDocument();
    expect(screen.getByTestId("icon")).toBeInTheDocument();
    fireEvent.click(screen.getByText("Section"));
    expect(screen.getByText("Body")).toBeInTheDocument();
  });
});
