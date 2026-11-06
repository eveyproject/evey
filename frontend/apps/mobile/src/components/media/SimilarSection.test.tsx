import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  FlatList: ({ data, renderItem }: any) =>
    React.createElement("div", null, (data ?? []).map((item: any, index: number) =>
      React.createElement(React.Fragment, { key: item.id ?? index }, renderItem({ item, index })))),
  Dimensions: { get: () => ({ width: 400, height: 800 }) },
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("expo-router", () => ({ router: { push: vi.fn() } }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff" }, spacing: { lg: 24, sm: 8 } }),
}));
const similar = vi.hoisted(() => ({ items: [] as unknown[] }));
vi.mock("@evey/api", () => ({
  useFeedServiceSimilarItems: () => ({ data: { items: similar.items } }),
}));
vi.mock("../MediaCard", () => ({
  MediaCard: ({ item }: { item: { id?: string; title?: string; year?: number; artwork?: { blurhash?: string }[] } }) =>
    React.createElement("div", { "data-testid": `card-${item.id}`, "data-blurhash": item.artwork?.[0]?.blurhash }, `${item.title}${item.year ? ` (${item.year})` : ""}`),
}));

import { SimilarSection } from "./SimilarSection";

describe("SimilarSection (mobile)", () => {
  // The server writes the year and the blurhash on the similar rail; the
  // projection used to keep only id and title.
  it("hands the card the year and the blurhash the feed carries", () => {
    similar.items = [{ mediaItemId: "s-1", title: "Similar A", year: 2001, primaryBlurhash: "LKO2" }, { title: "no id" }];
    render(<SimilarSection item={{ id: "m-1" } as never} />);
    expect(screen.getByTestId("card-s-1").textContent).toBe("Similar A (2001)");
    expect(screen.getByTestId("card-s-1").getAttribute("data-blurhash")).toBe("LKO2");
    expect(screen.queryByText(/no id/)).toBeNull();
  });
});
