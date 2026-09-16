import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

// When the media page cannot load, a title the phone already holds must
// still be playable from here: the button appears only once the local copy
// is confirmed on disk, next to Retry; a title that left the library keeps
// only the way back; no copy, no button.
vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityRole, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, role: accessibilityRole, "aria-label": accessibilityLabel }, children),
}));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("../../providers/ThemeProvider", () => ({ useTheme: () => ({ colors: { bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00", onAccent: "#000" }, spacing: { md: 16, xl: 32 }, radii: { md: 8 } }) }));
const getDownloadPath = vi.hoisted(() => vi.fn());
vi.mock("../../lib/downloads", () => ({ getDownloadPath: (...a: unknown[]) => getDownloadPath(...a) }));

import { MediaLoadFailed } from "./MediaLoadFailed";

beforeEach(() => {
  getDownloadPath.mockReset();
});

describe("MediaLoadFailed", () => {
  it("offers to play the downloaded copy once it is confirmed on disk", async () => {
    getDownloadPath.mockResolvedValue("/f/film.mkv");
    const onPlayLocal = vi.fn();
    const onRetry = vi.fn();
    render(<MediaLoadFailed mediaId="m1" gone={false} onRetry={onRetry} onBack={vi.fn()} onPlayLocal={onPlayLocal} />);
    expect(screen.queryByRole("button", { name: "media.detail.playDownloadedCopy" })).toBeNull();
    const play = await screen.findByRole("button", { name: "media.detail.playDownloadedCopy" });
    expect(getDownloadPath).toHaveBeenCalledWith("m1");
    fireEvent.click(play);
    expect(onPlayLocal).toHaveBeenCalledTimes(1);
    fireEvent.click(screen.getByRole("button", { name: "common.retry" }));
    expect(onRetry).toHaveBeenCalledTimes(1);
    expect(screen.getByText("common.loadError")).toBeTruthy();
  });

  it("shows no play button without a copy, and only the way back for a title that left the library", async () => {
    getDownloadPath.mockResolvedValue(null);
    const first = render(<MediaLoadFailed mediaId="m1" gone={false} onRetry={vi.fn()} onBack={vi.fn()} onPlayLocal={vi.fn()} />);
    await waitFor(() => expect(getDownloadPath).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("button", { name: "media.detail.playDownloadedCopy" })).toBeNull();
    first.unmount();

    getDownloadPath.mockResolvedValue("/f/film.mkv");
    const onBack = vi.fn();
    render(<MediaLoadFailed mediaId="m1" gone onRetry={vi.fn()} onBack={onBack} onPlayLocal={vi.fn()} />);
    expect(screen.getByText("common.mediaGone")).toBeTruthy();
    fireEvent.click(screen.getByRole("button", { name: "common.back" }));
    expect(onBack).toHaveBeenCalledTimes(1);
    await waitFor(() => expect(getDownloadPath).toHaveBeenCalledTimes(2));
    expect(screen.queryByRole("button", { name: "media.detail.playDownloadedCopy" })).toBeNull();
  });

  it("swallows a refused lookup", async () => {
    getDownloadPath.mockRejectedValue(new Error("denied"));
    render(<MediaLoadFailed mediaId="m1" gone={false} onRetry={vi.fn()} onBack={vi.fn()} onPlayLocal={vi.fn()} />);
    await waitFor(() => expect(getDownloadPath).toHaveBeenCalledTimes(1));
    expect(screen.queryByRole("button", { name: "media.detail.playDownloadedCopy" })).toBeNull();
  });
});
