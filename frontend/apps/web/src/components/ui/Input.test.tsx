import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Input } from "./Input";

describe("Input", () => {
  it("renders an input", () => {
    render(<Input placeholder="Enter text" />);
    expect(screen.getByPlaceholderText("Enter text")).toBeInTheDocument();
  });

  it("fires onChange", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Input placeholder="type" onChange={onChange} />);
    await user.type(screen.getByPlaceholderText("type"), "abc");
    expect(onChange).toHaveBeenCalledTimes(3);
  });

  it("renders label linked to input", () => {
    render(<Input label="Username" />);
    expect(screen.getByLabelText("Username")).toBeInTheDocument();
  });

  it("renders disabled state", () => {
    render(<Input disabled placeholder="nope" />);
    expect(screen.getByPlaceholderText("nope")).toBeDisabled();
  });

  it("shows error message", () => {
    render(<Input error="Required field" />);
    expect(screen.getByText("Required field")).toBeInTheDocument();
  });

  it("applies error border class", () => {
    render(<Input error="bad" placeholder="e" />);
    expect(screen.getByPlaceholderText("e")).toHaveClass("border-[var(--danger-bg)]");
  });
});
