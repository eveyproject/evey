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

  it("gives every option the width of the longest label on wide screens and stacks them on a phone", () => {
    render(<SegmentedControl label="Share" options={options} value="" onChange={() => {}} />);
    const group = screen.getByRole("radiogroup");
    // Phone: one option per row, full width. Wide: equal columns (1fr) in a
    // content-sized grid, so every column is as wide as the longest label.
    for (const c of ["grid", "w-full", "grid-flow-row", "sm:inline-grid", "sm:w-auto", "sm:grid-flow-col", "sm:auto-cols-fr"]) {
      expect(group.className.split(" ")).toContain(c);
    }
    for (const r of within(group).getAllByRole("radio")) expect(r.className).toContain("text-center");
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

  it("while disabled, refuses picks but keeps the focus (aria-disabled, not disabled)", () => {
    const onChange = vi.fn();
    render(<SegmentedControl label="Share" options={options} value="" onChange={onChange} disabled />);
    for (const r of screen.getAllByRole("radio")) {
      expect(r).toHaveAttribute("aria-disabled", "true");
      expect(r).not.toBeDisabled();
    }
    fireEvent.click(screen.getByRole("radio", { name: "50 %" }));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("in manual activation the arrows only move the focus; Space or Enter (a click) picks", () => {
    const onChange = vi.fn();
    render(<SegmentedControl label="Share" options={options} value="25" onChange={onChange} activation="manual" />);
    const current = screen.getByRole("radio", { name: "25 %" });
    current.focus();
    fireEvent.keyDown(current, { key: "ArrowRight" });
    expect(onChange).not.toHaveBeenCalled();
    expect(document.activeElement).toBe(screen.getByRole("radio", { name: "50 %" }));
    fireEvent.click(document.activeElement as HTMLElement);
    expect(onChange).toHaveBeenCalledWith("50");
  });

  it("is named by its visible label when linked", () => {
    render(
      <>
        <span id="lbl">Memory profile</span>
        <SegmentedControl label="fallback" labelledBy="lbl" options={options} value="" onChange={() => {}} />
      </>,
    );
    expect(screen.getByRole("radiogroup", { name: "Memory profile" })).toBeInTheDocument();
  });

  it("shows a stored value no option offers instead of checking nothing", () => {
    render(<SegmentedControl label="Height" options={[{ value: "", label: "Auto" }, { value: "720", label: "720p" }]} value="900" onChange={() => {}} formatUnknown={(v) => `${v}p`} />);
    expect(screen.getByRole("radio", { checked: true })).toHaveTextContent("900p");
  });
});
