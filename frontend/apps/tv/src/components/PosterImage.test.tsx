import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { render, screen, fireEvent, act } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", { "data-testid": p.testID }, children),
  Pressable: ({ children }: any) => React.createElement("div", null, children),
}));
vi.mock("@expo/vector-icons/Ionicons", () => ({ default: ({ name }: any) => React.createElement("i", { "data-icon": name }) }));
vi.mock("../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { surface: "#111", textMuted: "#888" } }) }));
vi.mock("../lib/liteMode", () => ({ useLiteMode: () => false }));
vi.mock("expo-image", () => ({
  Image: ({ source, onError, onLoad }: any) => React.createElement("img", { src: source?.uri, onError, onLoad }),
}));

import { IMAGE_RETRY_DELAYS_MS, resetImageRetryForTests } from "@evey/client";
import { PosterImage } from "./PosterImage";

describe("PosterImage", () => {
  beforeEach(() => {
    vi.useFakeTimers();
    resetImageRetryForTests();
  });
  afterEach(() => vi.useRealTimers());

  it("retries a failed poster on a fresh URL, and shows the glyph only once every retry failed", () => {
    const { container } = render(<PosterImage uri="http://s/p.webp" style={{}} />);
    expect(screen.queryByTestId("poster-fallback")).toBeNull();
    fireEvent.error(container.querySelector("img")!);
    // Still the image (retrying), not the glyph.
    expect(screen.queryByTestId("poster-fallback")).toBeNull();
    act(() => vi.advanceTimersByTime(IMAGE_RETRY_DELAYS_MS[0]));
    expect(container.querySelector("img")?.getAttribute("src")).toBe("http://s/p.webp?r=1");
    for (const delay of IMAGE_RETRY_DELAYS_MS.slice(1)) {
      fireEvent.error(container.querySelector("img")!);
      act(() => vi.advanceTimersByTime(delay));
    }
    fireEvent.error(container.querySelector("img")!);
    expect(screen.getByTestId("poster-fallback")).toBeTruthy();
    expect(container.querySelector("i")?.getAttribute("data-icon")).toBe("film-outline");
  });

  it("tries again when the uri changes, and loads nothing for a relative uri (server not known yet)", () => {
    const { container, rerender } = render(<PosterImage uri="http://s/a" style={{}} />);
    fireEvent.error(container.querySelector("img")!);
    rerender(<PosterImage uri="http://s/b" style={{}} />);
    expect(container.querySelector("img")?.getAttribute("src")).toBe("http://s/b");
    rerender(<PosterImage uri="/api/v1/media/x/artwork/poster" style={{}} />);
    expect(container.querySelector("img")).toBeNull();
    expect(screen.queryByTestId("poster-fallback")).toBeNull();
  });
});
