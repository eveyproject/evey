import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  Pressable: ({ children, onPress, testID, accessibilityLabel, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "data-testid": testID, "aria-label": accessibilityLabel, ...p }, children),
  Modal: ({ children, visible }: any) => (visible ? React.createElement("div", { "data-testid": "modal" }, children) : null),
  FlatList: ({ data, renderItem, keyExtractor, ListEmptyComponent }: any) =>
    data.length === 0 && ListEmptyComponent
      ? ListEmptyComponent
      : React.createElement(
          "div",
          null,
          data.map((item: unknown, index: number) =>
            React.createElement(React.Fragment, { key: keyExtractor(item, index) }, renderItem({ item, index })),
          ),
        ),
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_k: string, fallback?: string) => fallback ?? _k }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00" },
    radii: { md: 8 },
  }),
}));

import { DirectoryPickerModal } from "./DirectoryPickerModal";

const DIRS = [
  { name: "movies", path: "/media/movies" },
  { name: "shows", path: "/media/shows" },
];

describe("DirectoryPickerModal", () => {
  it("selects a directory and closes", () => {
    const onSelect = vi.fn();
    const onClose = vi.fn();
    render(<DirectoryPickerModal visible dirs={DIRS} onSelect={onSelect} onClose={onClose} />);

    fireEvent.click(screen.getByTestId("dir-/media/shows"));

    expect(onSelect).toHaveBeenCalledWith("/media/shows");
    expect(onClose).toHaveBeenCalled();
  });

  it("shows the manual-path hint when the server lists no directories", () => {
    render(<DirectoryPickerModal visible dirs={[]} onSelect={vi.fn()} onClose={vi.fn()} />);
    expect(screen.getByText(/type a path manually/i)).toBeTruthy();
  });

  it("renders nothing when hidden", () => {
    render(<DirectoryPickerModal visible={false} dirs={DIRS} onSelect={vi.fn()} onClose={vi.fn()} />);
    expect(screen.queryByTestId("modal")).toBeNull();
  });
});
