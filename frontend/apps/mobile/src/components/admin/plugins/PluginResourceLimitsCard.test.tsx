import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "spinner" }),
  TextInput: ({ value, onChangeText }: any) =>
    React.createElement("input", { value: value ?? "", onChange: (e: React.ChangeEvent<HTMLInputElement>) => onChangeText?.(e.target.value) }),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => (typeof f === "string" ? f : _k) }) }));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const api = vi.hoisted(() => ({ limits: { data: undefined as unknown, isLoading: false }, update: vi.fn(), opts: null as any }));
vi.mock("@evey/api", () => ({
  useAdminPluginServiceGetPluginResourceLimits: () => api.limits,
  useAdminPluginServiceUpdatePluginResourceLimits: (opts: any) => {
    api.opts = opts;
    return { mutate: api.update, isPending: false };
  },
  getAdminPluginServiceGetPluginResourceLimitsQueryKey: () => ["limits"],
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000", border: "#333", bg: "#000" }, radii: { sm: 4, md: 8 } }),
}));

import { PluginResourceLimitsCard } from "./PluginResourceLimitsCard";

beforeEach(() => {
  vi.clearAllMocks();
  api.limits = { data: { memoryMb: 256, pidsMax: 64, diskMaxMb: 0, cpuNice: 5, sandboxMode: "strict" }, isLoading: false };
});

// The card seeds from the stored caps, sends every field as a number (a
// non-numeric entry reads as 0, not NaN) with the picked sandbox mode, and
// refreshes the caps once saved.
describe("PluginResourceLimitsCard", () => {
  it("seeds from the server and saves the edited caps as numbers with the picked mode", () => {
    render(<PluginResourceLimitsCard />);
    const inputs = document.querySelectorAll("input");
    expect(Array.from(inputs).map((i) => (i as HTMLInputElement).value)).toEqual(["256", "64", "0", "5"]);
    fireEvent.change(inputs[0], { target: { value: "512" } });
    fireEvent.change(inputs[3], { target: { value: "abc" } });
    fireEvent.click(screen.getByText("basic"));
    fireEvent.click(screen.getByText("Save"));
    expect(api.update).toHaveBeenCalledWith({ data: { memoryMb: 512, pidsMax: 64, diskMaxMb: 0, cpuNice: 0, sandboxMode: "basic" } });
    api.opts.mutation.onSuccess();
    expect(invalidate).toHaveBeenCalledWith({ queryKey: ["limits"] });
    expect(toast.success).toHaveBeenCalledWith("Resource limits saved");
    api.opts.mutation.onError();
    expect(toast.error).toHaveBeenCalledWith("Could not save resource limits");
  });

  it("shows a spinner while loading and defaults the mode to auto", () => {
    api.limits = { data: undefined, isLoading: true };
    render(<PluginResourceLimitsCard />);
    expect(screen.getByTestId("spinner")).toBeTruthy();
    api.limits = { data: { memoryMb: 0 }, isLoading: false };
    render(<PluginResourceLimitsCard />);
    fireEvent.click(screen.getByText("Save"));
    expect(api.update).toHaveBeenCalledWith({ data: { memoryMb: 0, pidsMax: 0, diskMaxMb: 0, cpuNice: 0, sandboxMode: "auto" } });
  });
});
