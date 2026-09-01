import { useState } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { RemotePlaybackMenu, type RemotePlaybackMenuProps } from "./RemotePlaybackMenu";

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

// The menu is a controlled component (open/onToggle owned by PlayerControls,
// so Escape/outside-click closing goes through the same panel machinery as
// Settings/Episodes). This harness reproduces that control loop for tests.
function Harness(props: Partial<RemotePlaybackMenuProps> = {}) {
  const [open, setOpen] = useState(props.open ?? false);
  return (
    <RemotePlaybackMenu
      mediaId={props.mediaId ?? "mi-abc"}
      getPositionMs={props.getPositionMs ?? (() => 0)}
      open={open}
      onToggle={() => setOpen((v) => !v)}
      castAvailable={props.castAvailable}
      castConnected={props.castConnected}
      remoteConnected={props.remoteConnected}
      castDeviceName={props.castDeviceName}
      onCastTrigger={props.onCastTrigger}
    />
  );
}

const TV_DEVICE = { id: "dev-tv", name: "Living Room TV", type: "tv", connectedAt: "2026-06-04T00:00:00Z" };
const PHONE_DEVICE = { id: "dev-phone", name: "My Phone", type: "mobile", connectedAt: "2026-06-04T00:01:00Z" };

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("RemotePlaybackMenu", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockDevices = [];
    mockIsLoading = false;
    mockIsPending = false;
  });

  // ── Visibility guard ──────────────────────────────────────────────────────────

  it("renders nothing when no cast path exists and the device list is empty", () => {
    const { container } = render(<Harness castAvailable={false} />);
    expect(container.innerHTML).toBe("");
  });

  it("renders the trigger when a Chromecast path exists, even with zero Evey devices", () => {
    render(<Harness castAvailable={true} />);
    expect(screen.getByRole("button", { name: /play on another device/i })).toBeInTheDocument();
  });

  it("renders the trigger when at least one Evey device is available, even with no cast path", () => {
    mockDevices = [TV_DEVICE];
    render(<Harness castAvailable={false} />);
    expect(screen.getByRole("button", { name: /play on another device/i })).toBeInTheDocument();
  });

  // ── SDK absent: PlayOn-only menu ───────────────────────────────────────────────

  it("SDK absent: lists only Evey devices, no Cast section, no empty section", async () => {
    mockDevices = [TV_DEVICE];
    render(<Harness castAvailable={false} />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));

    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
    expect(screen.queryByText(/^cast$/i)).toBeNull();
  });

  // ── Both sources present ──────────────────────────────────────────────────────

  it("lists both a Cast entry and Evey devices when both are available", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE];
    render(<Harness castAvailable={true} />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));

    expect(screen.getByRole("button", { name: /^cast$/i })).toBeInTheDocument();
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
    expect(screen.getByText(PHONE_DEVICE.name)).toBeInTheDocument();
  });

  it("clicking the Cast entry invokes onCastTrigger and closes the menu", async () => {
    const onCastTrigger = vi.fn();
    mockDevices = [TV_DEVICE];
    render(<Harness castAvailable={true} onCastTrigger={onCastTrigger} />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    await userEvent.click(screen.getByRole("button", { name: /^cast$/i }));

    expect(onCastTrigger).toHaveBeenCalledTimes(1);
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  // ── Casting state visible on the trigger ──────────────────────────────────────

  it("highlights the trigger and shows the device name while an SDK cast session is active", async () => {
    render(<Harness castAvailable={true} castConnected={true} castDeviceName="Living Room TV" />);
    const btn = screen.getByRole("button", { name: /play on another device/i });
    expect(btn).toHaveAttribute("title", "Living Room TV");

    await userEvent.click(btn);
    expect(screen.getByText("Living Room TV")).toBeInTheDocument();
  });

  it("highlights the trigger while a Remote Playback (fling) session is active, even without a device name", () => {
    render(<Harness castAvailable={true} remoteConnected={true} castDeviceName={null} />);
    const btn = screen.getByRole("button", { name: /play on another device/i });
    // No SDK device name for a fling session - the trigger still doesn't
    // pretend to know one, but stays visibly active (no title attribute set).
    expect(btn).not.toHaveAttribute("title");
  });

  // ── Popover open/close ────────────────────────────────────────────────────────

  it("popover is closed by default", () => {
    mockDevices = [TV_DEVICE];
    render(<Harness />);
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  it("opens the popover when the button is clicked", async () => {
    mockDevices = [TV_DEVICE];
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
  });

  it("sets aria-expanded=false when closed and true when open", async () => {
    mockDevices = [TV_DEVICE];
    render(<Harness />);
    const btn = screen.getByRole("button", { name: /play on another device/i });
    expect(btn).toHaveAttribute("aria-expanded", "false");
    await userEvent.click(btn);
    expect(btn).toHaveAttribute("aria-expanded", "true");
  });

  it("closes the popover when clicking the trigger button again", async () => {
    mockDevices = [TV_DEVICE];
    render(<Harness />);
    const btn = screen.getByRole("button", { name: /play on another device/i });
    await userEvent.click(btn);
    expect(screen.getByText(TV_DEVICE.name)).toBeInTheDocument();
    await userEvent.click(btn);
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  // ── Device list rendering ─────────────────────────────────────────────────────

  it("renders a list item for each Evey device", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE];
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    const items = screen.getAllByRole("listitem");
    expect(items).toHaveLength(2);
  });

  // ── Loading state inside popover ──────────────────────────────────────────────

  it("shows no device list items while loading", async () => {
    mockDevices = [];
    mockIsLoading = true;
    render(<Harness castAvailable={false} />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    expect(screen.queryByRole("listitem")).toBeNull();
  });

  // ── Empty state inside open popover ──────────────────────────────────────────

  it("shows the empty message when the Evey list is empty but a cast path keeps the popover reachable", async () => {
    render(<Harness castAvailable={true} />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    expect(screen.getByText(/no connected devices/i)).toBeInTheDocument();
  });

  // ── Selection dispatches command ──────────────────────────────────────────────

  it("calls mutate with the correct deviceId, mediaId and positionMs on device click", async () => {
    mockDevices = [TV_DEVICE];
    render(<Harness mediaId="mi-movie-1" getPositionMs={() => 12345} />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));
    expect(mockMutate).toHaveBeenCalledWith(
      {
        deviceId: TV_DEVICE.id,
        command: { type: "loadAndPlay", mediaId: "mi-movie-1", positionMs: 12345 },
      },
      expect.objectContaining({ onSuccess: expect.any(Function), onError: expect.any(Function) }),
    );
  });

  it("disables all device buttons while a command is pending", async () => {
    mockDevices = [TV_DEVICE, PHONE_DEVICE];
    mockIsPending = true;
    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    expect(screen.getByRole("button", { name: TV_DEVICE.name })).toBeDisabled();
    expect(screen.getByRole("button", { name: PHONE_DEVICE.name })).toBeDisabled();
  });

  it("shows success toast and closes popover after onSuccess", async () => {
    const { toast } = await import("@evey/store");
    mockDevices = [TV_DEVICE];
    mockMutate.mockImplementation((_args: unknown, callbacks: { onSuccess?: () => void }) => {
      callbacks?.onSuccess?.();
    });

    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));

    await waitFor(() => expect(toast.success).toHaveBeenCalled());
    expect(screen.queryByText(TV_DEVICE.name)).toBeNull();
  });

  it("shows error toast and calls refetch after onError", async () => {
    const { toast } = await import("@evey/store");
    mockDevices = [TV_DEVICE];
    mockMutate.mockImplementation((_args: unknown, callbacks: { onError?: () => void }) => {
      callbacks?.onError?.();
    });

    render(<Harness />);
    await userEvent.click(screen.getByRole("button", { name: /play on another device/i }));
    await userEvent.click(screen.getByRole("button", { name: TV_DEVICE.name }));

    await waitFor(() => expect(toast.error).toHaveBeenCalled());
    expect(mockRefetch).toHaveBeenCalled();
  });
});
