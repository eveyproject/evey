import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Checkbox } from "./Checkbox";

describe("Checkbox", () => {
  it("renders unchecked by default", () => {
    render(<Checkbox checked={false} onChange={vi.fn()} />);
    expect(screen.getByRole("checkbox")).not.toBeChecked();
  });

  it("renders checked state", () => {
    render(<Checkbox checked={true} onChange={vi.fn()} />);
    expect(screen.getByRole("checkbox")).toBeChecked();
  });

  it("calls onChange with new value on click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Checkbox checked={false} onChange={onChange} />);
    await user.click(screen.getByRole("checkbox"));
    expect(onChange).toHaveBeenCalledWith(true);
  });

  it("renders label text", () => {
    render(<Checkbox checked={false} onChange={vi.fn()} label="Accept terms" />);
    expect(screen.getByText("Accept terms")).toBeInTheDocument();
  });

  it("associates label with checkbox", () => {
    render(<Checkbox checked={false} onChange={vi.fn()} label="Accept" />);
    expect(screen.getByLabelText("Accept")).toBeInTheDocument();
  });

  it("renders disabled state", () => {
    render(<Checkbox checked={false} onChange={vi.fn()} disabled />);
    expect(screen.getByRole("checkbox")).toBeDisabled();
  });

  it("does not call onChange when disabled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Checkbox checked={false} onChange={onChange} disabled />);
    await user.click(screen.getByRole("checkbox"));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("applies disabled opacity class", () => {
    render(<Checkbox checked={false} onChange={vi.fn()} disabled label="Off" />);
    const label = screen.getByText("Off").closest("label");
    expect(label?.className).toContain("opacity-50");
  });
});
