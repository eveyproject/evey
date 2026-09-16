import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement("div", null, (data ?? []).map((item: any, i: number) => React.createElement(React.Fragment, { key: keyExtractor ? keyExtractor(item) : i }, renderItem({ item })))),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
  Dimensions: { get: () => ({ width: 400, height: 800 }) },
}));
vi.mock("expo-image", () => ({ Image: ({ source }: any) => React.createElement("img", { alt: "", src: source?.uri }) }));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ useRouter: () => router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => (typeof f === "string" ? f : _k) }) }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", surface: "#111", accent: "#f00" }, spacing: { sm: 8, md: 12 }, radii: { md: 8 } }),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10, sm: 12, base: 14, "3xl": 30 } }));

import { CollectionRow } from "./CollectionRow";

beforeEach(() => vi.clearAllMocks());

// A card opens the smart-collection screen when the collection has rules and
// the plain one otherwise; a collection without a poster shows its initial;
// an empty row renders nothing at all.
describe("CollectionRow", () => {
  it("routes smart and manual collections to their own screens", () => {
    render(
      <CollectionRow
        title="Sagas"
        collections={[
          { id: "c1", name: "Alien", posterUrl: "http://x/alien.jpg" },
          { id: "c2", name: "Weekend", rules: [{ field: "genre", operator: "eq", value: "Drame" }] },
        ] as any}
      />,
    );
    fireEvent.click(screen.getByLabelText("Alien"));
    expect(router.push).toHaveBeenLastCalledWith("/collections/c1");
    fireEvent.click(screen.getByLabelText("Weekend"));
    expect(router.push).toHaveBeenLastCalledWith("/collections/smart/c2");
    expect(screen.getByText("W")).toBeTruthy();
    expect(document.querySelector("img")?.getAttribute("src")).toBe("http://x/alien.jpg");
  });

  it("the see-all link goes where the caller says", () => {
    render(<CollectionRow title="Sagas" seeAllHref="/collections" collections={[{ id: "c1", name: "Alien" }] as any} />);
    fireEvent.click(screen.getByLabelText("See all"));
    expect(router.push).toHaveBeenCalledWith("/collections");
  });

  it("renders nothing without a collection", () => {
    const { container } = render(<CollectionRow title="Sagas" collections={[]} />);
    expect(container.innerHTML).toBe("");
  });
});
