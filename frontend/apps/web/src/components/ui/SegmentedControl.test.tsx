import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import { SegmentedControl } from "./SegmentedControl";

const options = [
  { value: "", label: "Follow the global share (25 %)" },
  { value: "25", label: "25 %" },
  { value: "50", label: "50 %" },
  { value: "80", label: "80 %" },
];

// A radio group of equal cells: every option gets the same width whatever its
// label (one grid of equal columns that wraps on a narrow screen), one option
// is checked, and the keyboard follows the ARIA radio group pattern.
describe("SegmentedControl", () => {
  it("is a named radio group with exactly one checked option", () => {
    render(<SegmentedControl label="Share" options={options} value="50" onChange={() => {}} />);
    const group = screen.getByRole("radiogroup", { name: "Share" });
    const radios = within(group).getAllByRole("radio");
    expect(radios).toHaveLength(4);
    expect(radios.filter((r) => r.getAttribute("aria-checked") === "true")).toHaveLength(1);
    expect(within(group).getByRole("radio", { checked: true })).toHaveTextContent("50 %");
    // One tab stop: the checked option.
    expect(radios.map((r) => r.tabIndex)).toEqual([-1, -1, 0, -1]);
  });

  it("gives every option the same width: equal grid columns that wrap", () => {
    render(<SegmentedControl label="Share" options={options} value="" onChange={() => {}} />);
    const group = screen.getByRole("radiogroup");
    expect(group.className).toContain("grid");
    expect(group.style.gridTemplateColumns).toBe("repeat(auto-fit, minmax(min(100%, 7rem), 1fr))");
    for (const r of within(group).getAllByRole("radio")) {
      expect(r.className).toContain("min-w-0");
      expect(r.className).toContain("text-center");
    }
  });

  it("picks on click, and moves with the arrow keys, Home and End", () => {
    const onChange = vi.fn();
    render(<SegmentedControl label="Share" options={options} value="25" onChange={onChange} />);
    fireEvent.click(screen.getByRole("radio", { name: "80 %" }));
    expect(onChange).toHaveBeenLastCalledWith("80");
    fireEvent.click(screen.getByRole("radio", { name: "25 %" }));
    expect(onChange).toHaveBeenCalledTimes(1); // the checked one is not re-sent

    const current = screen.getByRole("radio", { name: "25 %" });
    fireEvent.keyDown(current, { key: "ArrowRight" });
    expect(onChange).toHaveBeenLastCalledWith("50");
    fireEvent.keyDown(current, { key: "ArrowLeft" });
    expect(onChange).toHaveBeenLastCalledWith("");
    fireEvent.keyDown(current, { key: "End" });
    expect(onChange).toHaveBeenLastCalledWith("80");
    fireEvent.keyDown(current, { key: "Home" });
    expect(onChange).toHaveBeenLastCalledWith("");
  });

  it("disables every option when disabled", () => {
    render(<SegmentedControl label="Share" options={options} value="" onChange={() => {}} disabled />);
    for (const r of screen.getAllByRole("radio")) expect(r).toBeDisabled();
  });
});
