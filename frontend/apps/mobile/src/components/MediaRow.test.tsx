import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  FlatList: ({ data, renderItem, keyExtractor }: any) =>
    React.createElement("div", null, (data ?? []).map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item })))),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  StyleSheet: { create: (s: any) => s },
  Dimensions: { get: () => ({ width: 400, height: 800 }) },
}));
const router = vi.hoisted(() => ({ push: vi.fn() }));
vi.mock("expo-router", () => ({ router }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (_k: string, f?: string) => f ?? _k }) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { text: "#fff", accent: "#f00" }, spacing: { sm: 8, md: 12, lg: 16 } }) }));
vi.mock("../hooks/useWatchedMap", () => ({ useWatchedMap: () => new Map([["m1", true]]) }));
vi.mock("./MediaCard", () => ({
  MediaCard: ({ item, onPress, watchPercent, watched }: any) =>
    React.createElement("button", { "aria-label": item.title, onClick: onPress, "data-pct": watchPercent, "data-watched": String(!!watched) }),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10, lg: 18 } }));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: () => null }));

import { MediaRow } from "./MediaRow";

beforeEach(() => vi.clearAllMocks());

// A row hands each card its resume percentage and watched flag, reports the
// tapped item, routes "see all" where told, and hides when empty.
describe("MediaRow", () => {
  it("wires each card and reports taps", () => {
    const onItemPress = vi.fn();
    const items = [{ id: "m1", title: "One" }, { id: "m2", title: "Two" }] as any[];
    render(<MediaRow title="Recent" seeAllHref="/recent" items={items} onItemPress={onItemPress} watchPercents={{ m2: 40 }} />);
    expect(screen.getByText("Recent")).toBeTruthy();
    expect(screen.getByLabelText("One").getAttribute("data-watched")).toBe("true");
    expect(screen.getByLabelText("Two").getAttribute("data-watched")).toBe("false");
    expect(screen.getByLabelText("Two").getAttribute("data-pct")).toBe("40");
    fireEvent.click(screen.getByLabelText("Two"));
    expect(onItemPress).toHaveBeenCalledWith(items[1]);
    fireEvent.click(screen.getByLabelText("See all"));
    expect(router.push).toHaveBeenCalledWith("/recent");
  });

  it("opens the row's page from its title too, and a row without one keeps a plain title", () => {
    const items = [{ id: "m1", title: "One" }] as any[];
    render(<MediaRow title="Films" seeAllHref="/library/lib-1" items={items} />);
    fireEvent.click(screen.getByLabelText("Films"));
    expect(router.push).toHaveBeenCalledWith("/library/lib-1");
    render(<MediaRow title="Popular" items={items} />);
    expect(screen.queryByLabelText("Popular")).toBeNull();
    expect(screen.getByText("Popular")).toBeTruthy();
  });

  it("hides when empty and shows no see-all without a target", () => {
    const { container } = render(<MediaRow title="Recent" items={[]} />);
    expect(container.innerHTML).toBe("");
    render(<MediaRow title="Recent" items={[{ id: "m1", title: "One" }] as any[]} />);
    expect(screen.queryByLabelText("See all")).toBeNull();
  });
});
