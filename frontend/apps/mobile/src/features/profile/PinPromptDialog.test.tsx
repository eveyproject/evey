import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => React.createElement("span", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, disabled, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel }, children),
  TextInput: React.forwardRef(({ value, onChangeText, placeholder }: any, ref: any) => {
    React.useImperativeHandle(ref, () => ({ focus: () => {} }));
    return React.createElement("input", {
      placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    });
  }),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, fallback?: string) => (typeof fallback === "string" ? fallback : _k) }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { overlay: "#0008", surface: "#111", text: "#fff", textMuted: "#888", bg: "#000", border: "#333", error: "#f33", accent: "#f00", onAccent: "#000" },
    radii: { md: 8, lg: 12 },
    spacing: { xs: 4, sm: 8, md: 12, lg: 16, xl: 24 },
  }),
}));
const verifyMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  parentalServiceVerifyPin: (...args: unknown[]) => verifyMock(...args),
}));

import { PinPromptDialog } from "./PinPromptDialog";

const target = { id: "p-kid", name: "Kid" } as any;
const parent = { id: "p-parent", name: "Parent" } as any;

function typeAndConfirm(pin: string) {
  fireEvent.change(screen.getByPlaceholderText("----"), { target: { value: pin } });
  fireEvent.click(screen.getByLabelText("Confirm"));
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("PinPromptDialog", () => {
  it("keeps the entry to 4 digits and refuses to submit a short one", () => {
    render(<PinPromptDialog target={target} onCancel={vi.fn()} onVerified={vi.fn()} />);
    const input = screen.getByPlaceholderText("----") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "12a3456" } });
    expect(input.value).toBe("1234");
    fireEvent.change(input, { target: { value: "12" } });
    expect((screen.getByLabelText("Confirm") as HTMLButtonElement).disabled).toBe(true);
    expect(verifyMock).not.toHaveBeenCalled();
  });

  it("hands the proof to onVerified when the target accepts the PIN", async () => {
    verifyMock.mockResolvedValueOnce({ valid: true, pinToken: "proof-1" });
    const onVerified = vi.fn();
    render(<PinPromptDialog target={target} onCancel={vi.fn()} onVerified={onVerified} />);
    typeAndConfirm("1234");
    await waitFor(() => expect(onVerified).toHaveBeenCalledWith(target, "proof-1"));
    expect(verifyMock).toHaveBeenCalledWith("p-kid", { pin: "1234" });
  });

  // The profile being LEFT is asked first: verify-pin against a profile that
  // carries no PIN answers "valid" to any digits and mints a proof the switch
  // then refuses (@evey/client's verifyParentalPin).
  it("asks the profile being left before the target", async () => {
    verifyMock.mockResolvedValueOnce({ valid: true, pinToken: "proof-parent" });
    const onVerified = vi.fn();
    render(<PinPromptDialog target={target} fallback={parent} onCancel={vi.fn()} onVerified={onVerified} />);
    typeAndConfirm("1234");
    await waitFor(() => expect(onVerified).toHaveBeenCalledWith(target, "proof-parent"));
    expect(verifyMock).toHaveBeenNthCalledWith(1, "p-parent", { pin: "1234" });
  });

  it("says Incorrect PIN when both refuse, and clears the entry", async () => {
    verifyMock.mockResolvedValueOnce({ valid: false });
    verifyMock.mockResolvedValueOnce({ valid: false });
    const onVerified = vi.fn();
    render(<PinPromptDialog target={target} fallback={parent} onCancel={vi.fn()} onVerified={onVerified} />);
    typeAndConfirm("9999");
    await waitFor(() => expect(screen.getByText("Incorrect PIN")).toBeTruthy());
    expect(onVerified).not.toHaveBeenCalled();
    expect((screen.getByPlaceholderText("----") as HTMLInputElement).value).toBe("");
  });

  it("surfaces a real network failure instead of blaming the PIN", async () => {
    verifyMock.mockRejectedValueOnce(new Error("Network request failed"));
    render(<PinPromptDialog target={target} onCancel={vi.fn()} onVerified={vi.fn()} />);
    typeAndConfirm("1234");
    await waitFor(() => expect(screen.getByText("Network request failed")).toBeTruthy());
    expect(screen.queryByText("Incorrect PIN")).toBeNull();
  });
});
