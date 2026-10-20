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

import { MkvgoCard } from "./MkvgoCard";

const infoButtons = (root: HTMLElement) => root.querySelectorAll('[data-testid="setting-info-button"]');

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

  it("gives each of the two setting rows exactly one 'i' button", () => {
    const { container } = render(<MkvgoCard getEntryValue={getEntryValue({})} saveEntry={vi.fn()} saving={false} />);
    expect(infoButtons(container).length).toBe(2);
    expect(screen.getByRole("button", { name: "admin.settingsLayout.moreInfo:Ingest serving plan" })).toBeTruthy();
    expect(screen.getByRole("button", { name: "admin.settingsLayout.moreInfo:Playability shadow" })).toBeTruthy();
  });
});
