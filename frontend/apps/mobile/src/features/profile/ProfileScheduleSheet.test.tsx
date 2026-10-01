import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", null, children) : null),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, disabled, accessibilityLabel, accessibilityRole, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "aria-label": accessibilityLabel, role: accessibilityRole, "aria-checked": accessibilityState?.checked }, children),
  Switch: ({ value, onValueChange, accessibilityLabel }: any) =>
    React.createElement("input", { type: "checkbox", role: "switch", checked: value, "aria-label": accessibilityLabel, onChange: () => onValueChange(!value) }),
  TextInput: ({ value, accessibilityLabel }: any) => React.createElement("input", { value, readOnly: true, "aria-label": accessibilityLabel }),
  ActivityIndicator: () => React.createElement("div", null, "…"),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k, i18n: { language: "en" } }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", surface: "#111", surfaceRaised: "#222", border: "#333", onAccent: "#fff", warning: "#fa0", success: "#0a0", error: "#f33" },
    radii: { sm: 4, md: 8, lg: 12 },
    spacing: { sm: 8, md: 12 },
  }),
}));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast, useDemoStore: (sel: (s: { demoMode: boolean }) => unknown) => sel({ demoMode: false }) }));
const proposal = {
  enabled: true,
  mode: "week" as const,
  rules: [
    { days: [1, 2, 3, 4, 5], ranges: [{ start: 1080, end: 1320 }] },
    { days: [6, 0], ranges: [{ start: 540, end: 720 }, { start: 840, end: 1260 }] },
  ],
};
const api = vi.hoisted(() => ({ put: vi.fn() }));
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: { isKids: true, schedule: null, proposal, access: { open: true, overridden: false }, timezone: "Europe/Paris" } }),
  useQueryClient: () => ({ setQueryData: vi.fn() }),
}));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  updateProfileSchedule: (...a: unknown[]) => api.put(...a),
}));

import { ProfileScheduleSheet } from "./ProfileScheduleSheet";

beforeEach(() => api.put.mockReset());

// The phone edits the same rules as the web: the proposal is offered off,
// enabling and saving sends it as is.
describe("ProfileScheduleSheet", () => {
  it("shows the proposal and saves it once enabled", async () => {
    const onClose = vi.fn();
    api.put.mockResolvedValue({});
    render(<ProfileScheduleSheet profileId="kid" profileName="Léo" onClose={onClose} />);
    expect(screen.getByText("Monday to Friday")).toBeTruthy();
    expect((screen.getAllByLabelText("From")[0] as HTMLInputElement).value).toBe("18:00");

    fireEvent.click(screen.getByRole("switch"));
    await act(async () => {
      fireEvent.click(screen.getByLabelText("Save"));
    });
    expect(api.put).toHaveBeenCalledWith("kid", proposal);
    expect(onClose).toHaveBeenCalled();
  });
});
