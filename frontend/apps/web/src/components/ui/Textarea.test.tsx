import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { Textarea } from "./Textarea";

describe("Textarea", () => {
  it("renders textarea", () => {
    render(<Textarea />);
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("passes placeholder prop", () => {
    render(<Textarea placeholder="Write here..." />);
    expect(screen.getByPlaceholderText("Write here...")).toBeInTheDocument();
  });

  it("fires onChange", async () => {
    const user = userEvent.setup();
    const onChange = vi.fn();
    render(<Textarea onChange={onChange} />);
    await user.type(screen.getByRole("textbox"), "hi");
    expect(onChange).toHaveBeenCalled();
  });

  it("renders label linked to textarea", () => {
    render(<Textarea label="Description" />);
    expect(screen.getByLabelText("Description")).toBeInTheDocument();
  });

  it("shows error message", () => {
    render(<Textarea error="Too long" />);
    expect(screen.getByText("Too long")).toBeInTheDocument();
  });

  it("applies error border class", () => {
    render(<Textarea error="bad" placeholder="e" />);
    expect(screen.getByPlaceholderText("e")).toHaveClass("border-red-500");
  });

  it("renders with custom rows", () => {
    render(<Textarea rows={6} placeholder="r" />);
    expect(screen.getByPlaceholderText("r")).toHaveAttribute("rows", "6");
  });
});
