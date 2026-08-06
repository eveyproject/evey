import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

// The mobile StreamInfoPanel used to show a bare "-" for video bitrate when
// the probe only reports a container total, unlike web's estimated
// total-minus-audio row; ignored the served audioBitrate override; and
// showed the raw, noisy float frame rate (23.976023976023978) instead of a
// human-rounded one. This proves the three are now aligned with web.

// --- React Native mock -----------------------------------------------------------------
vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

// --- BottomSheet -------------------------------------------------------------------------
vi.mock("./BottomSheet", () => ({
  BottomSheet: ({ visible, children }: any) => (visible ? React.createElement("div", null, children) : null),
}));

// --- i18next ------------------------------------------------------------------------------
vi.mock("react-i18next", () => ({
  useTranslation: () => ({ t: (_key: string, fallback?: string) => fallback ?? _key }),
}));

// --- ThemeProvider --------------------------------------------------------------------
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", text: "#fff", textMuted: "#888", accent: "#f00",
      success: "#0f0", warning: "#ff0", divider: "#333",
    },
  }),
}));

import { StreamInfoPanel } from "./StreamInfoPanel";

describe("StreamInfoPanel - video bitrate estimate", () => {
  it("shows an estimated total-minus-audio bitrate when the video stream reports none of its own", () => {
    render(
      <StreamInfoPanel
        visible
        onClose={vi.fn()}
        streamInfo={{
          playMethod: "PLAY_METHOD_DIRECT_PLAY",
          bitrate: "5000000",
          mediaStreams: [
            { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264" },
            { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", bitrate: "192000" },
          ],
        }}
      />,
    );

    expect(screen.getByText("≈ 4.8 Mbps")).toBeTruthy();
  });
});

describe("StreamInfoPanel - served audio bitrate fallback", () => {
  it("prefers the served streamInfo.audioBitrate over the source stream's own bitrate", () => {
    render(
      <StreamInfoPanel
        visible
        onClose={vi.fn()}
        streamInfo={{
          playMethod: "PLAY_METHOD_TRANSCODE",
          audioBitrate: "128000",
          mediaStreams: [
            { type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", bitrate: "1500000" },
          ],
        }}
      />,
    );

    expect(screen.getByText("128 Kbps")).toBeTruthy();
    expect(screen.queryByText("1.5 Mbps")).toBeNull();
  });
});

describe("StreamInfoPanel - frame rate rounding", () => {
  it("rounds a noisy floating-point frame rate to 3 decimals", () => {
    render(
      <StreamInfoPanel
        visible
        onClose={vi.fn()}
        streamInfo={{
          playMethod: "PLAY_METHOD_DIRECT_PLAY",
          mediaStreams: [
            { type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", frameRate: 23.976023976023978 },
          ],
        }}
      />,
    );

    expect(screen.getByText("23.976 fps")).toBeTruthy();
  });
});
