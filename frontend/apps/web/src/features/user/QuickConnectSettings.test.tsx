import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockMutate = vi.fn();
let onSuccessCb: (() => void) | null = null;
let onErrorCb: (() => void) | null = null;

vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  useUserServiceAuthorizeQuickConnect: (opts: {
    mutation: { onSuccess: () => void; onError: () => void };
  }) => {
    onSuccessCb = opts.mutation.onSuccess;
    onErrorCb = opts.mutation.onError;
    return { mutate: mockMutate, isPending: false };
  },
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (k: string) => k }),
}));

describe("QuickConnectSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    onSuccessCb = null;
    onErrorCb = null;
  });

  it("renders title and description", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    expect(screen.getByText("auth.settings.quickConnect.title")).toBeInTheDocument();
    expect(screen.getByText("auth.settings.quickConnect.description")).toBeInTheDocument();
  });

  it("renders code input and authorize button", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    expect(screen.getByPlaceholderText("A1B2C3")).toBeInTheDocument();
    expect(screen.getByText("auth.settings.quickConnect.authorize")).toBeInTheDocument();
  });

  it("authorize button disabled when code is empty", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    const btn = screen.getByText("auth.settings.quickConnect.authorize");
    expect(btn).toBeDisabled();
  });

  it("authorize button enabled when code has 4+ chars", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    const input = screen.getByPlaceholderText("A1B2C3");
    fireEvent.change(input, { target: { value: "AB12CD" } });
    const btn = screen.getByText("auth.settings.quickConnect.authorize");
    expect(btn).not.toBeDisabled();
  });

  it("calls mutate with uppercase code on submit", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    const input = screen.getByPlaceholderText("A1B2C3");
    fireEvent.change(input, { target: { value: "ab12cd" } });

    const form = input.closest("form")!;
    fireEvent.submit(form);

    expect(mockMutate).toHaveBeenCalledWith({ data: { code: "AB12CD" } });
  });

  it("clears input on success", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    const input = screen.getByPlaceholderText("A1B2C3") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "AB12CD" } });

    // The success callback resets the input state: call it inside act.
    act(() => onSuccessCb?.());

    await waitFor(() => {
      expect(input.value).toBe("");
    });
  });

  it("shows toast on error", async () => {
    const { toast } = await import("@evey/store");
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);

    onErrorCb?.();

    expect(toast.error).toHaveBeenCalledWith("auth.settings.quickConnect.invalidCode");
  });

  it("uppercases input as user types", async () => {
    const { QuickConnectSettings } = await import("./QuickConnectSettings");
    render(<QuickConnectSettings />);
    const input = screen.getByPlaceholderText("A1B2C3") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "abc" } });
    expect(input.value).toBe("ABC");
  });
});
