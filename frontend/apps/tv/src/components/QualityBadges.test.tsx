import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", { "data-testid": p.testID }, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  StyleSheet: { create: (s: any) => s },
}));
vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { surfaceRaised: "#222", text: "#fff", accent: "#f60", onAccent: "#000" }, radii: { sm: 4 } }),
}));

import { QualityBadges } from "./QualityBadges";

describe("QualityBadges", () => {
  it("reads the resolution and HDR off the video stream", () => {
    render(<QualityBadges streams={[
      { type: "MEDIA_STREAM_TYPE_AUDIO" },
      { type: "MEDIA_STREAM_TYPE_VIDEO", width: 3840, height: 2160, colorTransfer: "smpte2084" },
    ]} />);
    expect(screen.getByText("4K")).toBeTruthy();
    expect(screen.getByText("HDR")).toBeTruthy();
  });

  it("shows the resolution alone on an SDR stream", () => {
    render(<QualityBadges streams={[{ type: "MEDIA_STREAM_TYPE_VIDEO", width: 1920, height: 1080, colorTransfer: "bt709" }]} />);
    expect(screen.getByText("1080p")).toBeTruthy();
    expect(screen.queryByText("HDR")).toBeNull();
  });

  it("draws nothing without a video stream (a series)", () => {
    render(<QualityBadges streams={undefined} />);
    expect(screen.queryByTestId("quality-badges")).toBeNull();
  });
});
