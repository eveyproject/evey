import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { FilterChip } from "./FilterChip";

describe("FilterChip", () => {
  it("renders children text", () => {
    render(<FilterChip active={false} onClick={vi.fn()}>Genre</FilterChip>);
    expect(screen.getByText("Genre")).toBeInTheDocument();
  });

  it("calls onClick when clicked", async () => {
    const user = userEvent.setup();
    const onClick = vi.fn();
    render(<FilterChip active={false} onClick={onClick}>Action</FilterChip>);
    await user.click(screen.getByRole("button"));
    expect(onClick).toHaveBeenCalledOnce();
  });

  it("applies active style", () => {
    render(<FilterChip active={true} onClick={vi.fn()}>Active</FilterChip>);
    const btn = screen.getByRole("button");
    expect(btn.style.backgroundColor).toBe("var(--accent)");
    expect(btn.style.color).toBe("var(--filter-active-text)");
  });

  it("applies inactive style", () => {
    render(<FilterChip active={false} onClick={vi.fn()}>Inactive</FilterChip>);
    const btn = screen.getByRole("button");
    expect(btn.style.backgroundColor).toBe("var(--surface-raised)");
    expect(btn.style.color).toBe("var(--text-muted)");
  });

  it("applies sm size classes by default", () => {
    render(<FilterChip active={false} onClick={vi.fn()}>Small</FilterChip>);
    const btn = screen.getByRole("button");
    expect(btn.className).toContain("px-2");
    expect(btn.className).toContain("text-xs");
  });

  it("applies md size classes", () => {
    render(<FilterChip active={false} onClick={vi.fn()} size="md">Medium</FilterChip>);
    const btn = screen.getByRole("button");
    expect(btn.className).toContain("px-3");
    expect(btn.className).toContain("text-sm");
  });

  it("renders icon when provided", () => {
    render(
      <FilterChip active={false} onClick={vi.fn()} icon={<span data-testid="icon">*</span>}>
        With Icon
      </FilterChip>,
    );
    expect(screen.getByTestId("icon")).toBeInTheDocument();
    expect(screen.getByRole("button").className).toContain("flex");
  });

  it("forwards extra className", () => {
    render(<FilterChip active={false} onClick={vi.fn()} className="extra">X</FilterChip>);
    expect(screen.getByRole("button").className).toContain("extra");
  });
});

// Selection is otherwise carried by background and border alone: a screen
// reader announced every chip identically whether or not it was the active
// filter, and so did anyone who cannot separate the two backgrounds. The chip
// is shared by the Library filters and the Search scope, so both screens were
// affected.
describe("FilterChip - the selected state is announced, not only painted", () => {
  it("marks the active chip as pressed", () => {
    render(<FilterChip active onClick={() => {}}>Movies</FilterChip>);
    expect(screen.getByRole("button", { name: "Movies" })).toHaveAttribute("aria-pressed", "true");
  });

  it("marks an inactive chip as not pressed - absent would read as 'not a toggle'", () => {
    render(<FilterChip active={false} onClick={() => {}}>Series</FilterChip>);
    expect(screen.getByRole("button", { name: "Series" })).toHaveAttribute("aria-pressed", "false");
  });
});
