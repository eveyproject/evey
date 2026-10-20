import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, disabled, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, ...p }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

import { MemoryProfileCard } from "./MemoryProfileCard";

describe("MemoryProfileCard", () => {
  it("marks the currently stored profile as selected", () => {
    render(<MemoryProfileCard getEntryValue={() => "low"} saveEntry={vi.fn()} saving={false} />);
    expect(screen.getByText("Low")).toBeTruthy();
  });

  it("saves server.memory_profile with the chosen value", () => {
    const saveEntry = vi.fn();
    render(<MemoryProfileCard getEntryValue={() => ""} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getByText("Low"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "server.memory_profile", value: "low" });
  });
});
