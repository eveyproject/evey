import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

const { announceMock, verifyPinMock } = vi.hoisted(() => ({
  announceMock: vi.fn(),
  verifyPinMock: vi.fn(),
}));

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: React.forwardRef(({ value, onChangeText, accessibilityLabel, placeholder }: any, ref: any) => {
    React.useImperativeHandle(ref, () => ({ focus: () => {} }));
    return React.createElement("input", {
      "aria-label": accessibilityLabel ?? placeholder,
      value: value ?? "",
      onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value),
    });
  }),
  Pressable: ({ children, onPress, disabled }: any) =>
    React.createElement("button", { onClick: onPress, disabled }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "loading", ...p }),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  StyleSheet: { create: (s: any) => s },
  AccessibilityInfo: { announceForAccessibility: announceMock },
}));

vi.mock("@evey/client", () => ({ verifyParentalPin: (...a: unknown[]) => verifyPinMock(...a) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { overlay: "#0008", surface: "#111", text: "#fff", textMuted: "#888", bg: "#000", border: "#333", error: "#f00", accent: "#00f" },
    radii: { lg: 12, md: 8 },
  }),
}));
vi.mock("../hooks/useParentalGate", () => ({ markUnlocked: vi.fn() }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { lg: 18, sm: 12, xs: 10, "3xl": 30 } }));

import { ParentalPINModal } from "./ParentalPINModal";

describe("ParentalPINModal accessibility", () => {
  beforeEach(() => {
    announceMock.mockClear();
    verifyPinMock.mockReset();
  });

  it("announces an incorrect PIN to the screen reader", async () => {
    verifyPinMock.mockResolvedValue(null);
    render(<ParentalPINModal visible onClose={() => {}} onVerified={() => {}} profileId="p1" />);

    fireEvent.change(screen.getByLabelText("Parental PIN"), { target: { value: "1234" } });
    fireEvent.click(screen.getByText("Verify"));

    await waitFor(() => expect(announceMock).toHaveBeenCalledWith("Incorrect PIN"));
  });

  it("does not announce anything on a correct PIN", async () => {
    verifyPinMock.mockResolvedValue("");
    const onVerified = vi.fn();
    render(<ParentalPINModal visible onClose={() => {}} onVerified={onVerified} profileId="p1" />);

    fireEvent.change(screen.getByLabelText("Parental PIN"), { target: { value: "1234" } });
    fireEvent.click(screen.getByText("Verify"));

    await waitFor(() => expect(onVerified).toHaveBeenCalled());
    expect(announceMock).not.toHaveBeenCalled();
  });

  // The gate is fail-closed by project decision, and its catch block is the
  // only thing standing between "the server did not answer" and "come in". A
  // regression that called onVerified() from that catch - opening restricted
  // content whenever the network hiccups - passed every test until this one.
  // A dead network is not a wrong PIN: the browser has always said so, this
  // modal used to announce "Incorrect PIN" for it.
  it("says the request failed when the server did not answer", async () => {
    verifyPinMock.mockImplementation(() => Promise.reject(new Error("network down")));
    const onVerified = vi.fn();
    render(<ParentalPINModal visible onClose={() => {}} onVerified={onVerified} profileId="p1" />);

    fireEvent.change(screen.getByLabelText("Parental PIN"), { target: { value: "1234" } });
    fireEvent.click(screen.getByText("Verify"));

    await waitFor(() => expect(announceMock).toHaveBeenCalledWith("Something went wrong."));
    expect(onVerified).not.toHaveBeenCalled();
    // And the field is cleared, so the failed attempt cannot be re-submitted
    // by a stray second tap.
    expect((screen.getByLabelText("Parental PIN") as HTMLInputElement).value).toBe("");
  });
});
