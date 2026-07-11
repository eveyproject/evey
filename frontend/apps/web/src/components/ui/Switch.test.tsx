import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Switch } from "./Switch";

describe("Switch", () => {
  it("renders unchecked state", () => {
    render(<Switch checked={false} onChange={vi.fn()} label="Enable feature" />);
    const el = screen.getByRole("switch");
    expect(el).toHaveAttribute("aria-checked", "false");
  });

  it("renders checked state", () => {
    render(<Switch checked={true} onChange={vi.fn()} label="Enable feature" />);
    const el = screen.getByRole("switch");
    expect(el).toHaveAttribute("aria-checked", "true");
  });

  it("calls onChange on click", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Switch checked={false} onChange={onChange} label="Enable feature" />);
    await user.click(screen.getByRole("switch"));
    expect(onChange).toHaveBeenCalledTimes(1);
  });

  it("does not call onChange when disabled", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Switch checked={false} onChange={onChange} label="Enable feature" disabled />);
    await user.click(screen.getByRole("switch"));
    expect(onChange).not.toHaveBeenCalled();
  });

  it("renders disabled state", () => {
    render(<Switch checked={false} onChange={vi.fn()} label="Enable feature" disabled />);
    expect(screen.getByRole("switch")).toBeDisabled();
  });

  it("exposes aria-label", () => {
    render(<Switch checked={false} onChange={vi.fn()} label="Enable feature" />);
    expect(screen.getByLabelText("Enable feature")).toBeInTheDocument();
  });

  it("forwards data-testid", () => {
    render(<Switch checked={false} onChange={vi.fn()} label="Enable feature" data-testid="my-switch" />);
    expect(screen.getByTestId("my-switch")).toBeInTheDocument();
  });
});
