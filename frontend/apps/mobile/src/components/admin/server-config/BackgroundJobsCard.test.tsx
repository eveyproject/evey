import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: ({ onChangeText, value, testID, ...p }: any) =>
    React.createElement("input", {
      "data-testid": testID,
      value: value ?? "",
      onChange: (e: any) => onChangeText?.(e.target.value),
      ...p,
    }),
  Switch: ({ value, onValueChange, testID, disabled }: any) =>
    React.createElement("input", {
      type: "checkbox",
      "data-testid": testID,
      checked: !!value,
      disabled,
      onChange: () => onValueChange?.(!value),
    }),
  TouchableOpacity: ({ children, onPress, disabled, testID, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID, ...p }, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00",
      onAccent: "#fff", text: "#fff", textMuted: "#888", warning: "#fa0",
    },
    radii: { sm: 4 },
  }),
}));

import { BackgroundJobsCard } from "./BackgroundJobsCard";

const saveEntry = vi.fn();
const deleteEntry = vi.fn();

beforeEach(() => {
  saveEntry.mockClear();
  deleteEntry.mockClear();
});

describe("BackgroundJobsCard", () => {
  it("turning a worker off writes the sentinel 'off' value", () => {
    render(<BackgroundJobsCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("bg-toggle-transcoding.level_backfill"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "transcoding.level_backfill", value: "off" });
  });

  it("turning a worker back on deletes the entry to revert to the server default", () => {
    render(
      <BackgroundJobsCard
        entries={[{ key: "transcoding.level_backfill", value: "off" }]}
        saveEntry={saveEntry}
        deleteEntry={deleteEntry}
        saving={false}
      />,
    );
    fireEvent.click(screen.getByTestId("bg-toggle-transcoding.level_backfill"));
    expect(deleteEntry).toHaveBeenCalledWith("transcoding.level_backfill");
    expect(saveEntry).not.toHaveBeenCalled();
  });

  it("saving pacing writes filled fields and deletes cleared ones", () => {
    render(
      <BackgroundJobsCard
        entries={[{ key: "transcoding.level_backfill_batch", value: "10" }]}
        saveEntry={saveEntry}
        deleteEntry={deleteEntry}
        saving={false}
      />,
    );
    fireEvent.change(screen.getByTestId("bg-rate-transcoding.level_backfill_batch"), { target: { value: "25" } });
    fireEvent.click(screen.getByTestId("bg-rates-save"));

    expect(saveEntry).toHaveBeenCalledWith(
      { key: "transcoding.level_backfill_batch", value: "25" },
      expect.anything(),
    );
    // The five untouched (empty) fields revert to the server default.
    expect(deleteEntry).toHaveBeenCalledWith("transcoding.keyframe_backfill_batch", expect.anything());
  });

  it("save stays disabled until a pacing field is edited", () => {
    render(<BackgroundJobsCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("bg-rates-save"));
    expect(saveEntry).not.toHaveBeenCalled();
    expect(deleteEntry).not.toHaveBeenCalled();
  });
});
