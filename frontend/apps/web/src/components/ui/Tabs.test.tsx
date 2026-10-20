import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { Tabs } from "./Tabs";

const tabs = ["a", "b", "c"].map((id) => ({ id, label: id.toUpperCase() }));

// The section menu: one tab stop, arrows move and open, and the open tab is
// scrolled into view (on a phone the row scrolls sideways).
describe("Tabs", () => {
  it("brings the open tab into view on mount and on change", () => {
    const scroll = vi.fn();
    const original = Element.prototype.scrollIntoView;
    Element.prototype.scrollIntoView = scroll;
    try {
      const { rerender } = render(<Tabs label="Sections" idPrefix="t" tabs={tabs} value="c" onChange={() => {}} />);
      expect(scroll).toHaveBeenCalledTimes(1);
      expect(scroll.mock.contexts[0]).toBe(screen.getByRole("tab", { name: "C" }));
      rerender(<Tabs label="Sections" idPrefix="t" tabs={tabs} value="a" onChange={() => {}} />);
      expect(scroll.mock.contexts[1]).toBe(screen.getByRole("tab", { name: "A" }));
    } finally {
      Element.prototype.scrollIntoView = original;
    }
  });

  it("links each tab to its panel and moves with the arrows", () => {
    const onChange = vi.fn();
    render(<Tabs label="Sections" idPrefix="t" tabs={tabs} value="a" onChange={onChange} />);
    const a = screen.getByRole("tab", { name: "A" });
    expect(a).toHaveAttribute("aria-controls", "t-panel-a");
    expect(a).toHaveAttribute("tabindex", "0");
    expect(screen.getByRole("tab", { name: "B" })).toHaveAttribute("tabindex", "-1");
    fireEvent.keyDown(a, { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith("c");
  });
});
