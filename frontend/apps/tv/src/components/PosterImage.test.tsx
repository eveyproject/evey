import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

// A poster the server cannot serve (404) must leave a film glyph on the card
// surface, not an empty rectangle; a poster that loads stays an image.
vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
}));
vi.mock("@expo/vector-icons", () => ({ Ionicons: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { surface: "#111", textMuted: "#888" } }) }));
vi.mock("expo-image", () => ({
  Image: ({ source, onError }: any) => React.createElement("img", { alt: "", src: source.uri, onError }),
}));

import { PosterImage } from "./PosterImage";

describe("PosterImage", () => {
  it("shows the image, then the fallback glyph once it fails", () => {
    const { container } = render(<PosterImage uri="http://s/p.webp" style={{}} />);
    expect(screen.queryByTestId("poster-fallback")).toBeNull();
    fireEvent.error(container.querySelector("img")!);
    expect(screen.getByTestId("poster-fallback")).toBeTruthy();
    expect(container.querySelector("i")?.getAttribute("data-icon")).toBe("film-outline");
  });

  it("tries again when the uri changes", () => {
    const { container, rerender } = render(<PosterImage uri="http://s/a" style={{}} />);
    fireEvent.error(container.querySelector("img")!);
    rerender(<PosterImage uri="http://s/b" style={{}} />);
    expect(screen.queryByTestId("poster-fallback")).toBeNull();
  });
});
