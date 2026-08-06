import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

const alertMock = vi.fn();

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
  TouchableOpacity: ({ children, onPress, disabled, testID, ...p }: any) =>
    React.createElement("button", { onClick: onPress, disabled, "data-testid": testID, ...p }, children),
  Pressable: ({ children, onPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p }, children),
  Alert: { alert: (...a: unknown[]) => alertMock(...a) },
  StyleSheet: { create: (s: any) => s },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", surfaceRaised: "#222", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888", error: "#f33" },
    radii: { sm: 4 },
  }),
}));

import { ConfigEntriesCard } from "./ConfigEntriesCard";

const saveEntry = vi.fn();
const deleteEntry = vi.fn();

beforeEach(() => {
  saveEntry.mockClear();
  deleteEntry.mockClear();
  alertMock.mockClear();
});

const ENTRIES = [{ key: "transcoding.some_flag", value: "off" }];

describe("ConfigEntriesCard", () => {
  it("edits an entry in place and saves the new value", () => {
    render(<ConfigEntriesCard entries={ENTRIES} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("entry-editbtn-transcoding.some_flag"));
    fireEvent.change(screen.getByTestId("entry-edit-transcoding.some_flag"), { target: { value: "on" } });
    fireEvent.click(screen.getByTestId("entry-save-transcoding.some_flag"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "transcoding.some_flag", value: "on" });
  });

  it("deletes only after the destructive alert is confirmed", () => {
    alertMock.mockImplementation((_t: string, _m: string, buttons: { text: string; onPress?: () => void }[]) => {
      buttons.find((b) => b.text === "Delete")?.onPress?.();
    });
    render(<ConfigEntriesCard entries={ENTRIES} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("entry-delete-transcoding.some_flag"));
    expect(deleteEntry).toHaveBeenCalledWith("transcoding.some_flag");
  });

  it("adds a new trimmed entry and requires a key", () => {
    render(<ConfigEntriesCard entries={[]} saveEntry={saveEntry} deleteEntry={deleteEntry} saving={false} />);
    fireEvent.click(screen.getByTestId("entry-add"));
    expect(saveEntry).not.toHaveBeenCalled();

    fireEvent.change(screen.getByTestId("entry-new-key"), { target: { value: " my.key " } });
    fireEvent.change(screen.getByTestId("entry-new-value"), { target: { value: "42" } });
    fireEvent.click(screen.getByTestId("entry-add"));
    expect(saveEntry).toHaveBeenCalledWith({ key: "my.key", value: "42" }, expect.anything());
  });
});
