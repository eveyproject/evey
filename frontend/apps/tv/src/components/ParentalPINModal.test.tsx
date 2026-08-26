import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

const { announceForAccessibilityMock } = vi.hoisted(() => ({
  announceForAccessibilityMock: vi.fn(),
}));
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  ActivityIndicator: (p: any) => React.createElement("div", p, "loading"),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  StyleSheet: { create: (s: any) => s },
  AccessibilityInfo: { announceForAccessibility: announceForAccessibilityMock },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", overlay: "rgba(0,0,0,.5)", border: "#333", error: "#f00" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

const markUnlockedMock = vi.fn();
vi.mock("../hooks/useParentalGate", () => ({
  markUnlocked: (...args: unknown[]) => markUnlockedMock(...args),
}));

const eveyFetchMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => eveyFetchMock(...args),
}));

import { ParentalPINModal, shouldTryFallback } from "./ParentalPINModal";

function enterPin(getByLabelText: (l: string) => HTMLElement, digits: string) {
  for (const d of digits) {
    fireEvent.click(getByLabelText(d));
  }
}

describe("shouldTryFallback", () => {
  it("is false when no fallback profile is configured", () => {
    expect(shouldTryFallback("prof-target", undefined)).toBe(false);
  });
  it("is false when the fallback IS the profile that was just tried", () => {
    expect(shouldTryFallback("prof-target", "prof-target")).toBe(false);
  });
  it("is true when a distinct fallback profile is configured", () => {
    expect(shouldTryFallback("prof-target", "prof-source")).toBe(true);
  });
});

describe("ParentalPINModal - server pin proof threading", () => {
  beforeEach(() => {
    eveyFetchMock.mockReset();
    markUnlockedMock.mockReset();
    announceForAccessibilityMock.mockReset();
  });

  it("forwards the server pinToken to onVerified on a direct match", async () => {
    eveyFetchMock.mockResolvedValueOnce({ valid: true, pinToken: "proof-abc" });
    const onVerified = vi.fn();
    const { getByLabelText } = render(
      <ParentalPINModal visible onClose={vi.fn()} onVerified={onVerified} profileId="prof-target" />,
    );
    enterPin(getByLabelText, "1234");
    await waitFor(() => expect(onVerified).toHaveBeenCalledWith("proof-abc"));
    expect(eveyFetchMock).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/profiles/prof-target/parental/verify-pin" }),
    );
    expect(markUnlockedMock).toHaveBeenCalledWith("prof-target");
  });

  it("falls through to the fallback profile's PIN when the target carries none", async () => {
    // Target (e.g. an adult profile) has no PIN configured server-side and
    // rejects; the currently active kids profile is tried next since it is
    // the one that actually holds the PIN.
    eveyFetchMock
      .mockResolvedValueOnce({ valid: false })
      .mockResolvedValueOnce({ valid: true, pinToken: "proof-from-fallback" });
    const onVerified = vi.fn();
    const { getByLabelText } = render(
      <ParentalPINModal
        visible
        onClose={vi.fn()}
        onVerified={onVerified}
        profileId="prof-target"
        fallbackProfileId="prof-source"
      />,
    );
    enterPin(getByLabelText, "1234");
    await waitFor(() => expect(onVerified).toHaveBeenCalledWith("proof-from-fallback"));
    expect(eveyFetchMock).toHaveBeenNthCalledWith(
      1,
      expect.objectContaining({ url: "/api/v1/profiles/prof-target/parental/verify-pin" }),
    );
    expect(eveyFetchMock).toHaveBeenNthCalledWith(
      2,
      expect.objectContaining({ url: "/api/v1/profiles/prof-source/parental/verify-pin" }),
    );
  });

  it("never calls onVerified when both the target and the fallback reject", async () => {
    eveyFetchMock.mockResolvedValueOnce({ valid: false }).mockResolvedValueOnce({ valid: false });
    const onVerified = vi.fn();
    const { getByLabelText, findByText } = render(
      <ParentalPINModal
        visible
        onClose={vi.fn()}
        onVerified={onVerified}
        profileId="prof-target"
        fallbackProfileId="prof-source"
      />,
    );
    enterPin(getByLabelText, "1234");
    await findByText("Incorrect PIN");
    expect(onVerified).not.toHaveBeenCalled();
  });
});

describe("ParentalPINModal - non-visual feedback", () => {
  beforeEach(() => {
    eveyFetchMock.mockReset();
    markUnlockedMock.mockReset();
    announceForAccessibilityMock.mockReset();
  });

  it("announces digit progress, never the digit itself", async () => {
    eveyFetchMock.mockResolvedValueOnce({ valid: true, pinToken: "proof-abc" });
    const { getByLabelText } = render(
      <ParentalPINModal visible onClose={vi.fn()} onVerified={vi.fn()} profileId="prof-target" />,
    );
    fireEvent.click(getByLabelText("1"));
    fireEvent.click(getByLabelText("2"));

    const announced = announceForAccessibilityMock.mock.calls.map((c) => String(c[0]));
    expect(announced.length).toBeGreaterThan(0);
    for (const message of announced) {
      expect(message).not.toContain("1");
      expect(message).not.toContain("2");
    }
    // The i18n mock hands back the key itself since no fallback string is given.
    expect(announced).toContain("media.parental.pinDigitsEntered");
  });

  it("announces an incorrect PIN", async () => {
    eveyFetchMock.mockResolvedValueOnce({ valid: false });
    const { getByLabelText, findByText } = render(
      <ParentalPINModal visible onClose={vi.fn()} onVerified={vi.fn()} profileId="prof-target" />,
    );
    enterPin(getByLabelText, "1234");
    await findByText("Incorrect PIN");
    await waitFor(() => expect(announceForAccessibilityMock).toHaveBeenCalledWith("Incorrect PIN"));
  });
});
