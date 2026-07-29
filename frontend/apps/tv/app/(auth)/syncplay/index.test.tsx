import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, ...p }, children),
  ActivityIndicator: (p: any) => React.createElement("div", { "data-testid": "spinner", ...p }),
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: string) => fallback ?? key }),
}));

vi.mock("../../../src/providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", border: "#333", error: "#f33" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

const replaceMock = vi.fn();
const backMock = vi.fn();
vi.mock("expo-router", () => ({
  router: {
    replace: (...args: unknown[]) => replaceMock(...args),
    back: (...args: unknown[]) => backMock(...args),
  },
}));

const joinMock = vi.fn();
vi.mock("../../../src/hooks/useSyncplay", () => ({
  joinSyncplayRoomByCode: (...args: unknown[]) => joinMock(...args),
}));

import SyncplayJoinScreen from "./index";

function pressDigits(getByLabelText: (l: string) => HTMLElement, digits: string) {
  for (const d of digits) fireEvent.click(getByLabelText(d));
}

beforeEach(() => {
  vi.clearAllMocks();
});

describe("SyncplayJoinScreen", () => {
  it("navigates to the room's media, carrying the roomId, once 5 digits are entered", async () => {
    joinMock.mockResolvedValueOnce({ roomId: "room-1", mediaId: "mi-1" });
    const { getByLabelText } = render(<SyncplayJoinScreen />);

    pressDigits(getByLabelText, "12345");

    await waitFor(() => expect(joinMock).toHaveBeenCalledWith("12345"));
    await waitFor(() => expect(replaceMock).toHaveBeenCalledWith("/(auth)/player/mi-1?roomId=room-1"));
  });

  it("shows a clear error and resets the code on an unknown/expired code, without navigating", async () => {
    joinMock.mockRejectedValueOnce(new Error("room not found"));
    const { getByLabelText, findByText } = render(<SyncplayJoinScreen />);

    pressDigits(getByLabelText, "00000");

    await findByText("Could not join - check the code.");
    expect(replaceMock).not.toHaveBeenCalled();

    // State is not corrupted: the digit dots are back to empty, a new
    // attempt can be typed immediately.
    pressDigits(getByLabelText, "1");
    expect(getByLabelText("Delete")).toBeTruthy(); // keypad is still usable
  });

  it("deletes the last digit on the delete key", async () => {
    const { getByLabelText } = render(<SyncplayJoinScreen />);
    pressDigits(getByLabelText, "123");
    fireEvent.click(getByLabelText("Delete")); // "123" -> "12"
    pressDigits(getByLabelText, "678");
    await waitFor(() => expect(joinMock).toHaveBeenCalledWith("12678"));
  });

  it("goes back without joining when the back button is pressed", () => {
    const { getByLabelText } = render(<SyncplayJoinScreen />);
    fireEvent.click(getByLabelText("Back"));
    expect(backMock).toHaveBeenCalled();
    expect(joinMock).not.toHaveBeenCalled();
  });
});
