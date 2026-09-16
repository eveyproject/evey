import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  Pressable: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
  Modal: ({ children, visible }: any) => (visible ? children : null),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key) }),
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { surface: "#111", text: "#fff", textMuted: "#aaa", accent: "#88f", border: "#333", success: "#0a0", warning: "#fa0", onAccent: "#000", onSuccess: "#000", onWarning: "#000" },
  }),
}));
vi.mock("./FocusableCard", () => ({
  FocusableCard: ({ children, onPress, accessibilityLabel }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel }, children),
}));
vi.mock("@evey/theme", () => ({ FONT_SIZE: { sm: 12, base: 14, lg: 18, xl: 22 } }));

import { StreamInfoPanel } from "./StreamInfoPanel";

const session = {
  playMethod: "PLAY_METHOD_TRANSCODE",
  videoDecision: "copy",
  audioDecision: "transcode",
  container: "mkv",
  bitrate: "12500000",
  transcodeReasons: ["audio_codec_unsupported"],
  mediaStreams: [
    { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", width: 3840, height: 2160, bitrate: "11000000", frameRate: 23.976 },
    { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", channels: 6, language: "fre" },
    { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", channels: 2, language: "eng" },
  ],
} as any;

// The panel describes the session honestly: the effective method (an audio
// re-encode with video copied IS a transcode), per-stream decisions as
// badges, the SELECTED audio stream rather than the first one, the source
// container, human units, and the reasons under the right heading.
describe("StreamInfoPanel", () => {
  it("renders nothing while closed and only the close row without a session", () => {
    const { container } = render(<StreamInfoPanel visible={false} onClose={vi.fn()} streamInfo={session} />);
    expect(container.innerHTML).toBe("");
    const onClose = vi.fn();
    render(<StreamInfoPanel visible onClose={onClose} streamInfo={null} />);
    expect(screen.queryByText("Playback")).toBeNull();
    fireEvent.click(screen.getByLabelText("Close"));
    expect(onClose).toHaveBeenCalled();
  });

  it("describes the selected audio stream and the effective method", () => {
    render(<StreamInfoPanel visible onClose={vi.fn()} streamInfo={session} audioStreamIndex={2} />);
    expect(screen.getByText("player.streamInfo.methodTranscode")).toBeTruthy();
    expect(screen.getByText("MKV")).toBeTruthy();
    expect(screen.getByText("12.5 Mbps")).toBeTruthy();
    expect(screen.getByText("HEVC")).toBeTruthy();
    expect(screen.getByText("3840×2160")).toBeTruthy();
    expect(screen.getByText("11.0 Mbps")).toBeTruthy();
    expect(screen.getByText("23.976 fps")).toBeTruthy();
    expect(screen.getByText("AAC")).toBeTruthy();
    expect(screen.queryByText("EAC3")).toBeNull();
    expect(screen.getByText("2.0")).toBeTruthy();
    expect(screen.getByText("eng")).toBeTruthy();
    expect(screen.getByText("Copié")).toBeTruthy();
    expect(screen.getByText("Transcodé")).toBeTruthy();
    expect(screen.getByText("player.streamInfo.transcodeReasons")).toBeTruthy();
    expect(screen.getByText("audio codec unsupported")).toBeTruthy();
  });

  it("a remux heads its reasons with 'why not direct play' and defaults to the first audio stream", () => {
    render(<StreamInfoPanel visible onClose={vi.fn()} streamInfo={{ ...session, audioDecision: "copy", transcodeReasons: ["container_not_supported"] }} />);
    expect(screen.getByText("player.streamInfo.methodDirectStream")).toBeTruthy();
    expect(screen.getByText("player.streamInfo.noDirectPlayReasons")).toBeTruthy();
    expect(screen.getByText("EAC3")).toBeTruthy();
    expect(screen.getByText("5.1")).toBeTruthy();
    expect(screen.getAllByText("Copié")).toHaveLength(2);
  });
});
