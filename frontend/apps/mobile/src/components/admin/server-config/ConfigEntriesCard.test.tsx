import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

const alertMock = vi.hoisted(() => vi.fn());

vi.mock("react-native", async () => ({
  ...(await import("../../../test/rnDom")).rnDom,
  Alert: { alert: (...a: unknown[]) => alertMock(...a) },
}));
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
vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
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
