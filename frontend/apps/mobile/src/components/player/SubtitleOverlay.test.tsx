import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, waitFor } from "@testing-library/react";
import React from "react";
import { DEFAULT_SUBTITLE_STYLE } from "../../lib/subtitleStyle";

const eveyFetchRawMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
}));

// subtitleStyle.ts imports expo-secure-store at module scope for its
// persistence helpers - unused by this component, but the real module
// requires the native runtime and crashes outside it if not stubbed here.
vi.mock("expo-secure-store", () => ({
  getItemAsync: vi.fn(),
  setItemAsync: vi.fn(),
}));

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Text: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("span", { ...rest, "data-testid": "subtitle-text", "data-style": JSON.stringify(style) }, children as React.ReactNode),
  StyleSheet: { create: (s: Record<string, unknown>) => s },
  useWindowDimensions: () => ({ width: 400, height: 1000 }),
}));

import { SubtitleOverlay } from "./SubtitleOverlay";

const VTT = `WEBVTT

00:00:01.000 --> 00:00:03.000
Hello there

00:00:05.000 --> 00:00:07.000
Second line
`;

beforeEach(() => {
  eveyFetchRawMock.mockReset();
});

describe("SubtitleOverlay", () => {
  it("renders nothing when subtitleIndex is null", () => {
    const { container } = render(
      <SubtitleOverlay mediaId="m1" subtitleIndex={null} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    expect(container.innerHTML).toBe("");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });

  it("fetches the subtitle track and renders the cue active at currentTimeSec", async () => {
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve(VTT) });
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="m2" subtitleIndex={3} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    const text = await findByTestId("subtitle-text");
    expect(text.textContent).toBe("Hello there");
    expect(eveyFetchRawMock).toHaveBeenCalledWith({
      url: "/api/v1/media/m2/subtitle/3",
      method: "GET",
    });
  });

  it("renders nothing when currentTimeSec falls between cues", async () => {
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve(VTT) });
    const { container, rerender } = render(
      <SubtitleOverlay mediaId="m3" subtitleIndex={0} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await waitFor(() => expect(container.querySelector("[data-testid='subtitle-text']")).not.toBeNull());

    rerender(
      <SubtitleOverlay mediaId="m3" subtitleIndex={0} currentTimeSec={4} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    expect(container.querySelector("[data-testid='subtitle-text']")).toBeNull();
  });

  it("applies the resync offset before picking the active cue", async () => {
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve(VTT) });
    // Second cue is 5-7s; a -1000ms offset (subtitles earlier) shifts it to
    // 4-6s, so currentTimeSec=4.5 now falls inside it.
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="m4" subtitleIndex={0} currentTimeSec={4.5} offsetMs={-1000} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    const text = await findByTestId("subtitle-text");
    expect(text.textContent).toBe("Second line");
  });

  it("applies the style's text color and background", async () => {
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve(VTT) });
    const style = { ...DEFAULT_SUBTITLE_STYLE, color: "#fde047", bgColor: "#1e3a5f", bgOpacity: 50 };
    const { findByTestId } = render(
      <SubtitleOverlay mediaId="m5" subtitleIndex={0} currentTimeSec={2} style={style} />,
    );
    const text = await findByTestId("subtitle-text");
    const styleAttr = text.getAttribute("data-style");
    expect(styleAttr).toContain("#fde047");
    expect(styleAttr).toContain("rgba(30,58,95,0.5)");
  });

  it("caches cues per (mediaId, subtitleIndex) - a second mount does not re-fetch", async () => {
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve(VTT) });
    const { findByTestId, unmount } = render(
      <SubtitleOverlay mediaId="m-cache" subtitleIndex={0} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await findByTestId("subtitle-text");
    unmount();

    eveyFetchRawMock.mockClear();
    const { findByTestId: findByTestId2 } = render(
      <SubtitleOverlay mediaId="m-cache" subtitleIndex={0} currentTimeSec={2} style={DEFAULT_SUBTITLE_STYLE} />,
    );
    await findByTestId2("subtitle-text");
    expect(eveyFetchRawMock).not.toHaveBeenCalled();
  });
});
