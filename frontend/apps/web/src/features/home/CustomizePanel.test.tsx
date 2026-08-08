import { render, screen, fireEvent } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { CustomizePanel } from "./CustomizePanel";

const mockMutate = vi.fn();
const mockResetMutate = vi.fn();
let mockFeedData: { configs: Array<{ feedType: string; enabled: boolean; displayOrder: number }> } | undefined;
let mockLibrariesData: { libraries: Array<{ id: string; name: string }> } = {
  libraries: [{ id: "lib-1", name: "Movies" }, { id: "lib-2", name: "TV Shows" }],
};

vi.mock("@evey/api", () => ({
  useFeedServiceGetFeedConfig: () => ({ data: mockFeedData }),
  useFeedServiceUpdateProfileFeedConfig: () => ({ mutate: mockMutate }),
  useFeedServiceResetProfileFeedConfig: () => ({ mutate: mockResetMutate }),
  useLibraryServiceListLibraries: () => ({ data: mockLibrariesData }),
  // The panel imports eveyFetch too. Stubbed rather than inherited from the
  // real module so an untested save path fails loudly here instead of trying
  // to reach the network.
  eveyFetch: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (selector: (s: { profileId: string }) => unknown) =>
    selector({ profileId: "p1" }),
}));

vi.mock("lucide-react", () => {
  // Forward className/style/onPointerDown so tests can assert on the drag handle.
  const Icon = ({ size, ...rest }: { size?: number } & Record<string, unknown>) => (
    <span data-size={size} {...rest} />
  );
  return { X: Icon, GripVertical: Icon, RotateCcw: Icon, ChevronUp: Icon, ChevronDown: Icon };
});

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderPanel(open = true, onClose = vi.fn()) {
  return render(<CustomizePanel open={open} onClose={onClose} />, { wrapper: createWrapper() });
}

describe("CustomizePanel", () => {
  beforeEach(() => {
    mockMutate.mockReset();
    mockResetMutate.mockReset();
    mockFeedData = {
      configs: [
        { feedType: "resume_playback", enabled: true, displayOrder: 0 },
        { feedType: "recommended", enabled: false, displayOrder: 1 },
        { feedType: "most_popular", enabled: true, displayOrder: 2 },
      ],
    };
  });

  it("returns null when closed", () => {
    const { container } = renderPanel(false);
    expect(container.innerHTML).toBe("");
  });

  it("renders panel title", () => {
    renderPanel();
    expect(screen.getByText("Customize Homepage")).toBeInTheDocument();
  });

  it("renders all feed rows", () => {
    renderPanel();
    expect(screen.getByText("Resume Playback")).toBeInTheDocument();
    expect(screen.getByText("Recommended")).toBeInTheDocument();
    expect(screen.getByText("Most Popular")).toBeInTheDocument();
  });

  it("renders library feed with library name", () => {
    mockFeedData = {
      configs: [{ feedType: "library:lib-1", enabled: true, displayOrder: 0 }],
    };
    renderPanel();
    expect(screen.getByText("Movies")).toBeInTheDocument();
  });

  it("filters out unknown feed types (not in KNOWN_FEED_TYPES)", () => {
    mockFeedData = {
      configs: [{ feedType: "custom_unknown", enabled: true, displayOrder: 0 }],
    };
    renderPanel();
    expect(screen.queryByText("custom_unknown")).not.toBeInTheDocument();
  });

  it("toggle calls updateMutation with flipped enabled", async () => {
    const user = userEvent.setup();
    renderPanel();
    const toggleButtons = screen.getAllByRole("switch");
    await user.click(toggleButtons[0]);
    expect(mockMutate).toHaveBeenCalledWith(
      { feedType: "resume_playback", data: { enabled: false, displayOrder: 0 } },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("toggle disabled row calls updateMutation with enabled=true", async () => {
    const user = userEvent.setup();
    renderPanel();
    const toggleButtons = screen.getAllByRole("switch");
    await user.click(toggleButtons[1]);
    expect(mockMutate).toHaveBeenCalledWith(
      { feedType: "recommended", data: { enabled: true, displayOrder: 1 } },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("close button calls onClose", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    render(<CustomizePanel open={true} onClose={onClose} />, { wrapper: createWrapper() });
    const buttons = screen.getAllByRole("button");
    await user.click(buttons[0]);
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("Escape key calls onClose", () => {
    const onClose = vi.fn();
    render(<CustomizePanel open={true} onClose={onClose} />, { wrapper: createWrapper() });
    fireEvent.keyDown(document, { key: "Escape" });
    expect(onClose).toHaveBeenCalledOnce();
  });

  it("clicking outside panel calls onClose", () => {
    const onClose = vi.fn();
    const { container } = render(<CustomizePanel open={true} onClose={onClose} />, { wrapper: createWrapper() });
    const overlay = container.firstChild as HTMLElement;
    fireEvent.mouseDown(overlay);
    expect(onClose).toHaveBeenCalled();
  });

  it("reset button calls resetMutation for each row", async () => {
    const user = userEvent.setup();
    renderPanel();
    await user.click(screen.getByText("Reset to Defaults"));
    expect(mockResetMutate).toHaveBeenCalledTimes(3);
    expect(mockResetMutate).toHaveBeenCalledWith(
      { feedType: "resume_playback" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    expect(mockResetMutate).toHaveBeenCalledWith(
      { feedType: "recommended" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
    expect(mockResetMutate).toHaveBeenCalledWith(
      { feedType: "most_popular" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("rows have a pointer-drag handle (touch-capable reorder)", () => {
    renderPanel();
    // Reorder now uses Pointer Events (HTML5 native DnD has no touch support):
    // each row carries data-dragidx and a grip handle that opts out of native
    // touch scrolling via touch-action:none.
    const row = screen.getByText("Resume Playback").closest("[data-dragidx]");
    expect(row).not.toBeNull();
    const grip = row!.querySelector(".cursor-grab");
    expect(grip).not.toBeNull();
    expect(grip!.getAttribute("style")).toContain("touch-action: none");
  });

  it("sorts rows by displayOrder", () => {
    mockFeedData = {
      configs: [
        { feedType: "most_popular", enabled: true, displayOrder: 2 },
        { feedType: "resume_playback", enabled: true, displayOrder: 0 },
        { feedType: "recommended", enabled: true, displayOrder: 1 },
      ],
    };
    renderPanel();
    const rows = screen.getAllByText(/Resume Playback|Recommended|Most Popular/);
    expect(rows[0]).toHaveTextContent("Resume Playback");
    expect(rows[1]).toHaveTextContent("Recommended");
    expect(rows[2]).toHaveTextContent("Most Popular");
  });

  it("handles empty config data", () => {
    mockFeedData = undefined;
    const { container } = renderPanel();
    expect(screen.getByText("Customize Homepage")).toBeInTheDocument();
    expect(container.querySelectorAll("[draggable]")).toHaveLength(0);
  });

  it("inner panel has role=dialog with aria-label", () => {
    renderPanel();
    expect(
      screen.getByRole("dialog", { name: /Customize Homepage/i }),
    ).toBeInTheDocument();
  });
});

// The drag handle is pointer-only by nature (pointerdown + pointermove), so
// before these buttons a keyboard-only user could not reorder their homepage
// sections at all - not awkwardly, not at all. The translation catalogue even
// carried common.feeds.moveUp/moveDown, unused, from when this was planned.
describe("CustomizePanel keyboard reordering", () => {
  beforeEach(() => {
    mockFeedData = {
      configs: [
        { feedType: "resume_playback", enabled: true, displayOrder: 0 },
        { feedType: "recommended", enabled: true, displayOrder: 1 },
        { feedType: "most_popular", enabled: true, displayOrder: 2 },
      ],
    };
    mockMutate.mockClear();
  });

  it("moves a section down with a focusable, labelled control", () => {
    renderPanel();
    const down = screen.getAllByLabelText("common.feeds.moveDown");
    expect(down.length).toBe(3);
    // A <button> is reachable by Tab; a bare icon with onPointerDown is not.
    expect(down[0].tagName).toBe("BUTTON");

    fireEvent.click(down[0]);

    const rows = screen.getAllByText(/Resume Playback|Recommended|Most Popular/);
    expect(rows[0]).toHaveTextContent("Recommended");
    expect(rows[1]).toHaveTextContent("Resume Playback");
    // The new order is persisted, not just shown.
    expect(mockMutate).toHaveBeenCalled();
  });

  it("moves a section up", () => {
    renderPanel();
    fireEvent.click(screen.getAllByLabelText("common.feeds.moveUp")[2]);

    const rows = screen.getAllByText(/Resume Playback|Recommended|Most Popular/);
    expect(rows[1]).toHaveTextContent("Most Popular");
    expect(rows[2]).toHaveTextContent("Recommended");
  });

  // Without this the first row's "up" and the last row's "down" would reorder
  // into nothing and still fire a persist call.
  it("disables the moves that would fall off either end", () => {
    renderPanel();
    expect(screen.getAllByLabelText("common.feeds.moveUp")[0]).toBeDisabled();
    expect(screen.getAllByLabelText("common.feeds.moveDown")[2]).toBeDisabled();
  });
});
