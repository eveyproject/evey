import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Select } from "./Select";

const options = [
  { value: "a", label: "Option A" },
  { value: "b", label: "Option B" },
  { value: "c", label: "Option C" },
];

describe("Select", () => {
  it("renders options", () => {
    render(<Select options={options} value="a" onChange={vi.fn()} />);
    expect(screen.getByRole("combobox")).toBeInTheDocument();
    expect(screen.getByText("Option A")).toBeInTheDocument();
    expect(screen.getByText("Option B")).toBeInTheDocument();
    expect(screen.getByText("Option C")).toBeInTheDocument();
  });

  it("calls onChange with selected value", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Select options={options} value="a" onChange={onChange} />);
    await user.selectOptions(screen.getByRole("combobox"), "b");
    expect(onChange).toHaveBeenCalledWith("b");
  });

  it("renders label linked to select", () => {
    render(<Select label="Sort by" options={options} value="" onChange={vi.fn()} />);
    expect(screen.getByLabelText("Sort by")).toBeInTheDocument();
  });

  it("renders placeholder option", () => {
    render(<Select options={options} value="" placeholder="Pick one" onChange={vi.fn()} />);
    expect(screen.getByText("Pick one")).toBeInTheDocument();
  });

  it("shows error message", () => {
    render(<Select options={options} value="" error="Required" onChange={vi.fn()} />);
    expect(screen.getByText("Required")).toBeInTheDocument();
  });

  it("renders disabled state", () => {
    render(<Select options={options} value="a" disabled onChange={vi.fn()} />);
    expect(screen.getByRole("combobox")).toBeDisabled();
  });
});
