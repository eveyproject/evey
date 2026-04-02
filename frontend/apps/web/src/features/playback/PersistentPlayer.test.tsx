import { describe, it, expect, vi, beforeEach } from "vitest";
import { render } from "@testing-library/react";
import { PersistentPlayer } from "./PersistentPlayer";

let storeState: Record<string, unknown> = {};

vi.mock("@evey/store", () => ({
  usePlayerStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    if (typeof selector === "function") return selector(storeState);
    return storeState;
  },
}));

vi.mock("./MiniPlayer", () => ({
  MiniPlayer: () => <div data-testid="mini-player" />,
}));

beforeEach(() => {
  vi.clearAllMocks();
  storeState = { isActive: false, mode: "full" };
});

describe("PersistentPlayer", () => {
  it("renders nothing when not active", () => {
    const { container } = render(<PersistentPlayer />);
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when active but mode is not mini", () => {
    storeState = { isActive: true, mode: "full" };
    const { container } = render(<PersistentPlayer />);
    expect(container.innerHTML).toBe("");
  });

  it("renders MiniPlayer when active and mode is mini", () => {
    storeState = { isActive: true, mode: "mini" };
    const { getByTestId } = render(<PersistentPlayer />);
    expect(getByTestId("mini-player")).toBeInTheDocument();
  });
});
