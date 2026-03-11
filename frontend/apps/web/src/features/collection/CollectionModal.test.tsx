import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { CollectionModal } from "./CollectionModal";

vi.mock("@/components/ui/Select", () => ({
  Select: ({ label, value, onChange, options }: {
    label?: string;
    value: string;
    onChange: (v: string) => void;
    options: { value: string; label: string }[];
  }) => (
    <div>
      {label && <label>{label}</label>}
      <select value={value} onChange={(e) => onChange(e.target.value)} aria-label={label}>
        {options.map((o) => (
          <option key={o.value} value={o.value}>{o.label}</option>
        ))}
      </select>
    </div>
  ),
}));

const defaultProps = {
  open: true,
  onClose: vi.fn(),
  onSubmit: vi.fn(),
};

describe("CollectionModal", () => {
  it("renders create form", () => {
    render(<CollectionModal {...defaultProps} />);
    expect(screen.getByRole("dialog")).toBeInTheDocument();
    expect(screen.getByText("New Collection")).toBeInTheDocument();
    expect(screen.getByLabelText("Name")).toBeInTheDocument();
    expect(screen.getByLabelText("Description")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Create" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Cancel" })).toBeInTheDocument();
  });

  it("shows edit title when initial is provided", () => {
    const initial = {
      id: "1",
      name: "My Collection",
      type: "smart" as const,
      rules: [],
      sortBy: "sort_title",
      sortOrder: "asc",
      createdAt: "",
      updatedAt: "",
    };
    render(<CollectionModal {...defaultProps} initial={initial} />);
    expect(screen.getByText("Edit Collection")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Save" })).toBeInTheDocument();
  });

  it("submits with correct data", async () => {
    const user = userEvent.setup();
    const onSubmit = vi.fn();
    render(<CollectionModal {...defaultProps} onSubmit={onSubmit} />);

    await user.type(screen.getByLabelText("Name"), "Action Movies");

    const typeSelect = screen.getByLabelText("Type");
    await user.selectOptions(typeSelect, "manual");

    await user.click(screen.getByRole("button", { name: "Create" }));

    expect(onSubmit).toHaveBeenCalledWith(
      expect.objectContaining({
        name: "Action Movies",
        type: "manual",
        rules: [],
      })
    );
  });

  it("calls onClose on Cancel", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CollectionModal {...defaultProps} onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalled();
  });

  it("submit button disabled when name is empty", () => {
    render(<CollectionModal {...defaultProps} />);
    expect(screen.getByRole("button", { name: "Create" })).toBeDisabled();
  });

  it("shows rules section for smart type", () => {
    render(<CollectionModal {...defaultProps} />);
    expect(screen.getByText("Rules")).toBeInTheDocument();
    expect(screen.getByText("+ Add rule")).toBeInTheDocument();
  });
});
