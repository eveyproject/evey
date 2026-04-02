import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MiniPlayer } from "./MiniPlayer";

const mockStop = vi.fn();
const mockSetMode = vi.fn();
const mockNavigate = vi.fn();

vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

let storeState: Record<string, unknown> = {};

vi.mock("@evey/store", () => ({
  usePlayerStore: (selector?: (s: Record<string, unknown>) => unknown) => {
    if (typeof selector === "function") return selector(storeState);
    return storeState;
  },
}));

beforeEach(() => {
  vi.clearAllMocks();
  storeState = {
    mediaId: null,
    mediaTitle: null,
    videoElement: null,
    stop: mockStop,
    setMode: mockSetMode,
  };
});

describe("MiniPlayer", () => {
  it("renders nothing when no mediaId", () => {
    const { container } = render(<MiniPlayer />);
    expect(container.innerHTML).toBe("");
  });

  it("renders controls when mediaId is set", () => {
    storeState = { ...storeState, mediaId: "m1", mediaTitle: "Test Movie" };
    render(<MiniPlayer />);
    // videoElement is null so !undefined = true => "Pause" label
    expect(screen.getByLabelText("Pause")).toBeInTheDocument();
    expect(screen.getByLabelText("Expand")).toBeInTheDocument();
    expect(screen.getByLabelText("Close")).toBeInTheDocument();
    expect(screen.getByText("Test Movie")).toBeInTheDocument();
  });

  it("calls stop when close button clicked", async () => {
    storeState = { ...storeState, mediaId: "m1", mediaTitle: "Test" };
    render(<MiniPlayer />);
    await userEvent.click(screen.getByLabelText("Close"));
    expect(mockStop).toHaveBeenCalled();
  });
});
