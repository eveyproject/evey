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

import { MkvgoCard } from "./MkvgoCard";

describe("MkvgoCard", () => {
  function getEntryValue(map: Record<string, string>) {
    return (key: string) => map[key] ?? "";
  }

  it("saves the ingest serving plan key on selection", () => {
    const saveEntry = vi.fn();
    render(<MkvgoCard getEntryValue={getEntryValue({})} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getByText("Plan + repair"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "library.ingest_serving_plan", value: "plan+repair" });
  });

  it("saves the playability shadow key independently of the ingest plan", () => {
    const saveEntry = vi.fn();
    render(<MkvgoCard getEntryValue={getEntryValue({ "library.ingest_serving_plan": "plan" })} saveEntry={saveEntry} saving={false} />);
    // The ingest plan select has its own "Off" chip; the shadow's is the second.
    fireEvent.click(screen.getAllByText("Off")[1]);
    expect(saveEntry).toHaveBeenCalledWith({ key: "transcoding.playability_shadow", value: "off" });
  });
});
