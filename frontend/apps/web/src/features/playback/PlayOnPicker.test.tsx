import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PlayOnPicker } from "./PlayOnPicker";

// ── Mutable mock state ────────────────────────────────────────────────────────

const mockMutate = vi.fn();
const mockRefetch = vi.fn();

let mockDevices: unknown[] = [];
let mockIsLoading = false;
let mockIsPending = false;

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@/hooks/usePlayOnDevices", () => ({
  usePlayOnDevices: () => ({
    data: mockDevices,
    isLoading: mockIsLoading,
    refetch: mockRefetch,
  }),
  usePlayOnCommand: () => ({
    mutate: mockMutate,
    isPending: mockIsPending,
  }),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderPicker(props: { mediaId?: string; positionMs?: number } = {}) {
  return render(
    <PlayOnPicker mediaId={props.mediaId ?? "mi-abc"} positionMs={props.positionMs ?? 0} />,
    { wrapper: createWrapper() },
  );
}

const TV_DEVICE = { id: "dev-tv", name: "Living Room TV", type: "tv", connectedAt: "2026-06-04T00:00:00Z" };
const PHONE_DEVICE = { id: "dev-phone", name: "My Phone", type: "mobile", connectedAt: "2026-06-04T00:01:00Z" };
const DESKTOP_DEVICE = { id: "dev-desktop", name: "Home PC", type: "desktop", connectedAt: "2026-06-04T00:02:00Z" };

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("PlayOnPicker", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDevices = [];
    mockIsLoading = false;
    mockIsPending = false;
  });

  // ── Visibility guard ──────────────────────────────────────────────────────────

  it("renders nothing when device list is empty and not loading", () => {
    mockDevices = [];
    const { container } = renderPicker();
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when devices is undefined", () => {
    mockDevices = undefined as unknown as unknown[];
    const { container } = renderPicker();
    expect(container.innerHTML).toBe("");
  });

  it("renders the trigger button when at least one device is available", () => {
    mockDevices = [TV_DEVICE];
    renderPicker();
    expect(screen.getByRole("button", { name: /player\.playOn\.button/i })).toBeInTheDocument();
  });

  it("renders the trigger button while loading (before device list arrives)", () => {
    // isLoading=true, so the !isLoading && !hasDevices guard is false -> button renders
    mockDevices = [];
    mockIsLoading = true;
    renderPicker();
    expect(screen.getByRole("button", { name: /player\.playOn\.button/i })).toBeInTheDocument();
  });

  // ── Popover open/close ────────────────────────────────────────────────────────

  it("popover is closed by default", () => {
    mockDevices = [TV_DEVICE];
    renderPicker();
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  it("opens the popover when the button is clicked", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
  });

  it("sets aria-expanded=false when closed and true when open", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker();
    const btn = screen.getByRole("button", { name: /player\.playOn\.button/i });
    expect(btn).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(btn);
    expect(btn).toHaveAttribute("aria-expanded", "true");
  });

  it("closes the popover when clicking the trigger button again", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker();
    const btn = screen.getByRole("button", { name: /player\.playOn\.button/i });
    await userEvent.click(btn);
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
    await userEvent.click(btn);
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  it("closes the popover on outside click", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
    fireEvent.mouseDown(document.body);
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  // ── Device list rendering ─────────────────────────────────────────────────────

  it("renders all device names in the open popover", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE, DESKTOP_DEVICE];
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
    expect(screen.getByText(PHONE_DEVICE.name)).toBeInTheDocument();
    expect(screen.getByText(DESKTOP_DEVICE.name)).toBeInTheDocument();
  });

  it("renders a button for each device", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE];
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    expect(screen.getByRole("button", { name: TV_DEVICE.name })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: PHONE_DEVICE.name })).toBeInTheDocument();
  });

  it("renders a list item for each device", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE, DESKTOP_DEVICE];
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(3);
  });

  // ── Loading state inside popover ──────────────────────────────────────────────

  it("shows no device list items while loading", async () => {
    mockDevices = [];
    mockIsLoading = true;
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    expect(screen.queryByRole("listitem")).toBeNull();
  });

  // ── Empty state inside open popover ──────────────────────────────────────────

  it("shows the empty message when list is empty but popover is open", async () => {
    // Strategy: open popover while loading (shows spinner, no null-return since
    // isLoading=true bypasses the early-return guard), then switch isLoading to
    // false while keeping open=true via the same component instance.
    // rerender must use the same wrapper so component state is preserved.
    mockIsLoading = true;
    mockDevices = [];

    const qc = new QueryClient({ defaultOptions: { queries: { retry: false }, mutations: { retry: false } } });
    const Wrapper = ({ children }: { children: React.ReactNode }) => (
      <QueryClientProvider client={qc}>{children}</QueryClientProvider>
    );

    const { rerender } = render(
      <PlayOnPicker mediaId="mi-abc" positionMs={0} />,
      { wrapper: Wrapper },
    );

    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));

    // Now simulate devices arriving as empty — same component instance stays open
    mockIsLoading = false;
    rerender(<PlayOnPicker mediaId="mi-abc" positionMs={0} />);

    expect(screen.getByText(/player\.playOn\.empty/i)).toBeInTheDocument();
  });

  // ── Selection dispatches command ──────────────────────────────────────────────

  it("calls mutate with the correct deviceId, mediaId and positionMs on device click", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker({ mediaId: "mi-movie-1", positionMs: 12345 });
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));
    expect(mockMutate).toHaveBeenCalledWith(
      {
        deviceId: TV_DEVICE.id,
        command: { type: "loadAndPlay", mediaId: "mi-movie-1", positionMs: 12345 },
      },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
  });

  it("dispatches to the correct device when multiple are listed", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE];
    renderPicker({ mediaId: "mi-ep-1", positionMs: 0 });
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    await userEvent.click(screen.getByRole("button", { name: PHONE_DEVICE.name }));
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({ deviceId: PHONE_DEVICE.id }),
      expect.any(Object),
    );
  });

  it("uses loadAndPlay command type for every device selection", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker({ mediaId: "mi-abc", positionMs: 5000 });
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        command: expect.objectContaining({ type: "loadAndPlay" }),
      }),
      expect.any(Object),
    );
  });

  it("passes positionMs=0 when playback is at the start", async () => {
    mockDevices = [TV_DEVICE];
    renderPicker({ mediaId: "mi-abc", positionMs: 0 });
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));
    expect(mockMutate).toHaveBeenCalledWith(
      expect.objectContaining({
        command: expect.objectContaining({ positionMs: 0 }),
      }),
      expect.any(Object),
    );
  });

  // ── Pending state disables buttons ───────────────────────────────────────────

  it("disables all device buttons while a command is pending", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE];
    mockIsPending = true;
    renderPicker();
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    expect(screen.getByRole("button", { name: TV_DEVICE.name })).toBeDisabled();
    expect(screen.getByRole("button", { name: PHONE_DEVICE.name })).toBeDisabled();
  });

  // ── Success callback: toast + close ──────────────────────────────────────────

  it("shows success toast and closes popover after onSuccess", async () => {
    const { toast } = await import("@evey/store");
    mockDevices = [TV_DEVICE];
    mockMutate.mockImplementation((_args: unknown, callbacks: { onSuccess?: () => void }) => {
      callbacks?.onSuccess?.();
    });

    renderPicker({ mediaId: "mi-abc", positionMs: 0 });
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));

    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    // Popover closed after success
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  // ── Error callback: toast + refetch ──────────────────────────────────────────

  it("shows error toast and calls refetch after onError", async () => {
    const { toast } = await import("@evey/store");
    mockDevices = [TV_DEVICE];
    mockMutate.mockImplementation((_args: unknown, callbacks: { onError?: () => void }) => {
      callbacks?.onError?.();
    });

    renderPicker({ mediaId: "mi-abc", positionMs: 0 });
    await userEvent.click(screen.getByRole("button", { name: /player\.playOn\.button/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    // refetch triggered so disconnected device drops off the list
    expect(mockRefetch).toHaveBeenCalled();
  });
});
