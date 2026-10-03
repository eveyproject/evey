import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) => React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Dimensions: { get: () => ({ width: 400, height: 800 }) },
}));
vi.mock("expo-image", () => ({
  Image: ({ source, placeholder, onError }: any) => React.createElement("img", { alt: "", onError, src: source?.uri, "data-blurhash": placeholder?.blurhash }),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@evey/api", () => ({ displayTitle: (item: { title?: string; seriesTitle?: string }) => (item.seriesTitle ? `${item.seriesTitle} - ${item.title}` : item.title ?? "") }));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surface: "#111", text: "#fff", textMuted: "#888", success: "#0a0", onSuccess: "#000" }, spacing: { sm: 8 }, radii: { md: 8 } }),
}));
vi.mock("../hooks/useServerUrl", () => ({ useServerUrl: () => "http://evey.local" }));
vi.mock("../hooks/useAuthenticatedImage", () => ({ useAuthenticatedImage: (uri?: string) => (uri ? { uri, headers: { Authorization: "Bearer tok" } } : null) }));
vi.mock("./ProgressBar", () => ({ ProgressBar: ({ percent }: any) => React.createElement("div", { "data-testid": "progress", "data-pct": percent }) }));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { xs: 10 }, ICON_SIZE: { sm: 16, "3xl": 40 } }));

import { MediaCard } from "./MediaCard";

// The card asks for a display-sized poster through the authenticated image
// hook, seeds the placeholder with the primary blurhash (an explicit one
// from a feed item wins, the backdrop's is the fallback), shows the resume
// bar only past zero, and the watched badge only when told.
describe("MediaCard", () => {
  it("labels by display title, requests the poster and picks the blurhash", () => {
    const onPress = vi.fn();
    const item = { id: "m1", title: "Ep 2", seriesTitle: "Show", year: 2001, artwork: [{ type: "ARTWORK_TYPE_BACKDROP", blurhash: "bd" }, { type: "ARTWORK_TYPE_PRIMARY", blurhash: "pr" }] } as any;
    const { rerender } = render(<MediaCard item={item} onPress={onPress} watchPercent={40} watched />);
    fireEvent.click(screen.getByLabelText("Show - Ep 2"));
    expect(onPress).toHaveBeenCalled();
    const img = document.querySelector("img")!;
    expect(img.getAttribute("src")).toBe("http://evey.local/api/v1/media/m1/artwork/poster?w=400");
    expect(img.getAttribute("data-blurhash")).toBe("pr");
    expect(screen.getByTestId("progress").getAttribute("data-pct")).toBe("40");
    expect(document.querySelector("[data-icon='checkmark']")).toBeTruthy();
    expect(screen.getByText("2001")).toBeTruthy();

    rerender(<MediaCard item={item} posterBlurhash="feed" />);
    expect(document.querySelector("img")!.getAttribute("data-blurhash")).toBe("feed");
    expect(screen.queryByTestId("progress")).toBeNull();
    expect(document.querySelector("[data-icon='checkmark']")).toBeNull();
  });

  it("falls back to the backdrop blurhash, and shows no bar at zero progress nor a year without one", () => {
    render(<MediaCard item={{ id: "m2", title: "Film", artwork: [{ type: "ARTWORK_TYPE_BACKDROP", blurhash: "bd" }] } as any} watchPercent={0} />);
    expect(document.querySelector("img")!.getAttribute("data-blurhash")).toBe("bd");
    expect(screen.queryByTestId("progress")).toBeNull();
    expect(screen.getAllByText("Film")).toHaveLength(1);
  });

  it("swaps the empty rectangle for a film glyph when the poster fails to load", () => {
    render(<MediaCard item={{ id: "m9", title: "No poster" } as any} />);
    expect(document.querySelector("[data-icon='film-outline']")).toBeNull();
    fireEvent.error(document.querySelector("img")!);
    expect(document.querySelector("[data-icon='film-outline']")).toBeTruthy();
    expect(document.querySelector("img")).toBeNull();
  });
});
