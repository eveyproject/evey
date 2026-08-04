import React from "react";
import { render, screen, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const mockInitiateMutate = vi.fn();
let onSuccessCallback: ((data: { code?: string }) => void) | null = null;
// Mirrors the real hook's result shape: the component reads isError to decide
// between the spinner and a way out.
let mockIsError = false;

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceInitiateQuickConnect: (opts: { mutation: { onSuccess: (d: { code?: string }) => void } }) => {
    onSuccessCallback = opts.mutation.onSuccess;
    return { mutate: mockInitiateMutate, isError: mockIsError };
  },
}));

// Mock EventSource (SSE)
class MockEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  close = vi.fn();
}
vi.stubGlobal("EventSource", MockEventSource);

vi.mock("@evey/store", () => ({
  useAuthStore: () => ({ setTokens: vi.fn() }),
}));

const mockNavigate = vi.fn();
vi.mock("@tanstack/react-router", () => ({
  useNavigate: () => mockNavigate,
}));

vi.mock("react-qr-code", () => ({
  default: ({ value }: { value: string }) => <div data-testid="qr-code" data-value={value} />,
}));

vi.mock("@/components/ui/Spinner", () => ({
  Spinner: () => <div data-testid="spinner" />,
}));

describe("QuickConnect", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    vi.clearAllMocks();
    onSuccessCallback = null;
    mockIsError = false;
    mockInitiateMutate.mockImplementation(() => {
      // simulate async success - component calls mutate in useEffect
    });
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  // The request runs on mount, so a failure leaves no gesture to repeat: the
  // spinner spun for ever with nothing to click.
  it("offers a way out when the code request failed", async () => {
    mockIsError = true;
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);

    expect(screen.queryByTestId("spinner")).not.toBeInTheDocument();
    const retry = screen.getByTestId("quick-connect-retry");
    mockInitiateMutate.mockClear();
    act(() => { retry.click(); });
    expect(mockInitiateMutate).toHaveBeenCalledTimes(1);
  });

  it("shows spinner while code is being fetched", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });

  it("shows QR code when code is received via onSuccess", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);

    expect(screen.getByTestId("spinner")).toBeInTheDocument();

    await act(async () => {
      onSuccessCallback?.({ code: "ABCDEF123456" });
    });

    expect(screen.queryByTestId("spinner")).not.toBeInTheDocument();
    expect(screen.getByTestId("qr-code")).toBeInTheDocument();
  });

  it("shows last 6 chars of code uppercased", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);

    await act(async () => {
      onSuccessCallback?.({ code: "ABCDEF123456" });
    });

    // "ABCDEF123456".slice(-6).toUpperCase() === "123456"
    expect(screen.getByText("123456")).toBeInTheDocument();
  });

  it("uses 120s refresh interval", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);

    await act(async () => {
      onSuccessCallback?.({ code: "TESTCODE1234" });
    });

    expect(screen.getByText(/expiresIn/)).toBeInTheDocument();
  });

  it("shows countdown when code is present", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);

    await act(async () => {
      onSuccessCallback?.({ code: "TESTCODE1234" });
    });

    expect(screen.getByText(/expiresIn/)).toBeInTheDocument();
  });

  it("shows 'Scan with a logged-in device' instruction text", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);
    expect(screen.getByText(/scanInstruction/)).toBeInTheDocument();
  });

  it("does not show QR code initially (spinner shown)", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);
    expect(screen.queryByTestId("qr-code")).not.toBeInTheDocument();
    expect(screen.getByTestId("spinner")).toBeInTheDocument();
  });

  it("does not show expiry text without code", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);
    expect(screen.queryByText(/expiresIn/)).not.toBeInTheDocument();
  });
});
