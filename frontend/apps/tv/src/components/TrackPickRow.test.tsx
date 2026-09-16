import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, testID }: any) => React.createElement("div", { "data-testid": testID }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityState }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "aria-selected": String(!!accessibilityState?.selected) }, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: unknown) => {
      if (key === "media.track") return `Track ${(opts as { number: number }).number}`;
      return { "media.streams.auto": "Auto", "media.streams.none": "Off", "media.streams.forced": "forced", "media.streams.audio": "Audio", "media.streams.subtitles": "Subtitles" }[key] ?? key;
    },
  }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { textMuted: "#aaa", accent: "#88f", surface: "#111", onAccent: "#000", text: "#fff" } }),
}));

import { TrackPickRow } from "./TrackPickRow";

const streams = [
  { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO" },
  { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng", channels: 6 },
  { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", title: "Commentary", channels: 2 },
  { index: 3, type: "MEDIA_STREAM_TYPE_SUBTITLE", language: "fre", isForced: true },
] as any[];

// Audio and subtitle are chosen before playback: "Auto" is the untouched
// default and is what a page nobody touched sends; a pick replaces one half
// of the choice and leaves the other; a title with one audio track and no
// subtitles shows no row at all.
describe("TrackPickRow", () => {
  it("renders nothing when there is nothing to choose", () => {
    const { container } = render(<TrackPickRow streams={[streams[0], streams[1]]} pick={{}} onChange={vi.fn()} />);
    expect(container.innerHTML).toBe("");
  });

  it("labels the tracks and marks Auto until a pick is made", () => {
    const onChange = vi.fn();
    render(<TrackPickRow streams={streams} pick={{}} onChange={onChange} />);
    expect(screen.getByLabelText("Audio: Auto").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByLabelText("Audio: eng - 5.1")).toBeTruthy();
    expect(screen.getByLabelText("Audio: Commentary - 2.0")).toBeTruthy();
    expect(screen.getByLabelText("Subtitles: fre - forced")).toBeTruthy();
    expect(screen.getByLabelText("Subtitles: Auto").getAttribute("aria-selected")).toBe("true");

    fireEvent.click(screen.getByLabelText("Audio: Commentary - 2.0"));
    expect(onChange).toHaveBeenLastCalledWith({ audio: 2 });
    fireEvent.click(screen.getByLabelText("Subtitles: Off"));
    expect(onChange).toHaveBeenLastCalledWith({ subtitle: "off" });
  });

  it("a pick keeps the other half of the choice and can go back to Auto", () => {
    const onChange = vi.fn();
    render(<TrackPickRow streams={streams} pick={{ audio: 2, subtitle: 3 }} onChange={onChange} />);
    expect(screen.getByLabelText("Audio: Commentary - 2.0").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByLabelText("Subtitles: fre - forced").getAttribute("aria-selected")).toBe("true");
    expect(screen.getByLabelText("Audio: Auto").getAttribute("aria-selected")).toBe("false");
    fireEvent.click(screen.getByLabelText("Audio: Auto"));
    expect(onChange).toHaveBeenLastCalledWith({ audio: undefined, subtitle: 3 });
  });
});
