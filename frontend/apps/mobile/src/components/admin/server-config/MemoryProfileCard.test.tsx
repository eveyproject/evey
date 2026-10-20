import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => (await import("../../../test/rnDom")).rnDom);
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, o?: unknown) => {
      if (typeof o === "string") return o;
      if (o && typeof o === "object" && "label" in o) return `${key}:${String((o as { label: unknown }).label)}`;
      return key;
    },
  }),
}));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: new Proxy({}, { get: () => "#000" }), radii: { sm: 4, md: 8, lg: 12 } }),
}));

import { MemoryProfileCard } from "./MemoryProfileCard";

const infoButtons = (root: HTMLElement) => root.querySelectorAll('[data-testid="setting-info-button"]');

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

  it("leaves the 'i' and the name to its group title, and keeps the choice named for screen readers", () => {
    const { container } = render(<MemoryProfileCard getEntryValue={() => ""} saveEntry={vi.fn()} saving={false} />);
    expect(infoButtons(container).length).toBe(0);
    expect(screen.getByRole("radiogroup", { name: "Memory profile" })).toBeTruthy();
  });
});
