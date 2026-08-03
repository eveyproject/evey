import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { BulkActionBar } from "./BulkActionBar";

let mockIsAdmin = true;
vi.mock("@evey/store", () => ({
  useAuthStore: (sel: (s: { isAdmin: boolean }) => unknown) => sel({ isAdmin: mockIsAdmin }),
}));

const defaultProps = {
  count: 3,
  onClear: vi.fn(),
  onMarkWatched: vi.fn(),
  onMarkUnwatched: vi.fn(),
  onAddToPlaylist: vi.fn(),
  onAddToCollection: vi.fn(),
};

describe("BulkActionBar", () => {
  beforeEach(() => { mockIsAdmin = true; });

  // AddCollectionItem is RoleAdmin: a viewer offered it selected titles, opened
  // the picker and watched every choice fail. The rest of the bar is theirs.
  it("drops only the collection action for a viewer", () => {
    mockIsAdmin = false;
    render(<BulkActionBar {...defaultProps} />);
    expect(screen.queryByTestId("bulk-add-to-collection")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add to playlist/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /mark watched/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /mark unwatched/i })).toBeInTheDocument();
    expect(screen.getByText("3")).toBeInTheDocument();
  });

  it("shows selected count", () => {
    render(<BulkActionBar {...defaultProps} />);
    expect(screen.getByText("3")).toBeInTheDocument();
    expect(screen.getByText("selected")).toBeInTheDocument();
  });

  it("renders all action buttons", () => {
    render(<BulkActionBar {...defaultProps} />);
    expect(screen.getByRole("button", { name: /mark watched/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /mark unwatched/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add to playlist/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /add to collection/i })).toBeInTheDocument();
  });

  it("calls onClear when clear button clicked", async () => {
    const onClear = vi.fn();
    const user = userEvent.setup();
    render(<BulkActionBar {...defaultProps} onClear={onClear} />);
    await user.click(screen.getByRole("button", { name: /clear selection/i }));
    expect(onClear).toHaveBeenCalledOnce();
  });

  it("calls onMarkWatched when button clicked", async () => {
    const onMarkWatched = vi.fn();
    const user = userEvent.setup();
    render(<BulkActionBar {...defaultProps} onMarkWatched={onMarkWatched} />);
    await user.click(screen.getByRole("button", { name: /mark watched/i }));
    expect(onMarkWatched).toHaveBeenCalledOnce();
  });

  it("calls onMarkUnwatched when button clicked", async () => {
    const onMarkUnwatched = vi.fn();
    const user = userEvent.setup();
    render(<BulkActionBar {...defaultProps} onMarkUnwatched={onMarkUnwatched} />);
    await user.click(screen.getByRole("button", { name: /mark unwatched/i }));
    expect(onMarkUnwatched).toHaveBeenCalledOnce();
  });

  it("calls onAddToPlaylist when button clicked", async () => {
    const onAddToPlaylist = vi.fn();
    const user = userEvent.setup();
    render(<BulkActionBar {...defaultProps} onAddToPlaylist={onAddToPlaylist} />);
    await user.click(screen.getByRole("button", { name: /add to playlist/i }));
    expect(onAddToPlaylist).toHaveBeenCalledOnce();
  });

  it("calls onAddToCollection when button clicked", async () => {
    const onAddToCollection = vi.fn();
    const user = userEvent.setup();
    render(<BulkActionBar {...defaultProps} onAddToCollection={onAddToCollection} />);
    await user.click(screen.getByRole("button", { name: /add to collection/i }));
    expect(onAddToCollection).toHaveBeenCalledOnce();
  });

  it("disables buttons when count is 0", () => {
    render(<BulkActionBar {...defaultProps} count={0} />);
    expect(screen.getByRole("button", { name: /mark watched/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /mark unwatched/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /add to playlist/i })).toBeDisabled();
    expect(screen.getByRole("button", { name: /add to collection/i })).toBeDisabled();
  });

  it("has aria-live polite attribute", () => {
    const { container } = render(<BulkActionBar {...defaultProps} />);
    const el = container.querySelector("[aria-live='polite']");
    expect(el).toBeTruthy();
  });
});
