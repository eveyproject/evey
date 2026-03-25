import React from "react";
import { render, screen, waitFor, act } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";

const mockInitiateMutate = vi.fn();
let onSuccessCallback: ((data: { code?: string }) => void) | null = null;

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceInitiateQuickConnect: (opts: { mutation: { onSuccess: (d: { code?: string }) => void } }) => {
    onSuccessCallback = opts.mutation.onSuccess;
    return { mutate: mockInitiateMutate };
  },
  useUserServiceCheckQuickConnect: vi.fn(() => ({ data: undefined })),
}));

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
    mockInitiateMutate.mockImplementation(() => {
      // simulate async success - component calls mutate in useEffect
    });
  });

  afterEach(() => {
    vi.useRealTimers();
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

    // Should show "120" somewhere in the countdown
    expect(screen.getByText(/Expires in 120s/)).toBeInTheDocument();
  });

  it("shows countdown when code is present", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);

    await act(async () => {
      onSuccessCallback?.({ code: "TESTCODE1234" });
    });

    expect(screen.getByText(/Expires in/)).toBeInTheDocument();
  });

  it("shows 'Scan with a logged-in device' instruction text", async () => {
    const { QuickConnect } = await import("./QuickConnect");
    render(<QuickConnect />);
    expect(screen.getByText(/Scan with a logged-in device/)).toBeInTheDocument();
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
    expect(screen.queryByText(/Expires in/)).not.toBeInTheDocument();
  });
});
