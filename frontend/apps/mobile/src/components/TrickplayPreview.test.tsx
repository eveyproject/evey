import { describe, it, expect, vi, beforeEach } from "vitest";
import { act, render } from "@testing-library/react";
import React from "react";

const eveyFetchRawMock = vi.fn();
vi.mock("@evey/api", () => ({
  eveyFetch: vi.fn(),
  eveyFetchRaw: (...args: unknown[]) => eveyFetchRawMock(...args),
}));

vi.mock("../hooks/useServerUrl", () => ({
  useServerUrl: () => "http://srv",
}));

vi.mock("../hooks/useAuthenticatedImage", () => ({
  useAuthHeaders: () => ({ Authorization: "Bearer tok" }),
}));

vi.mock("expo-file-system/legacy", () => ({
  cacheDirectory: "/tmp/cache/",
  getInfoAsync: vi.fn().mockResolvedValue({ exists: false }),
  readAsStringAsync: vi.fn().mockResolvedValue("[]"),
  makeDirectoryAsync: vi.fn().mockResolvedValue(undefined),
  writeAsStringAsync: vi.fn().mockResolvedValue(undefined),
}));

vi.mock("expo-image", () => ({
  Image: (props: Record<string, unknown>) =>
    React.createElement("img", { "data-testid": "trickplay-img", src: (props.source as { uri: string })?.uri }),
}));

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  StyleSheet: { create: (s: Record<string, unknown>) => s },
}));

import { TrickplayPreview } from "./TrickplayPreview";

const VTT_SAMPLE = `WEBVTT

00:00:00.000 --> 00:00:10.000
/api/v1/media/m1/trickplay/tile0.jpg#xywh=0,0,160,90

00:00:10.000 --> 00:00:20.000
/api/v1/media/m1/trickplay/tile0.jpg#xywh=160,0,160,90

00:00:20.000 --> 00:00:30.000
/api/v1/media/m1/trickplay/tile1.jpg#xywh=0,0,160,90
`;

beforeEach(() => {
  eveyFetchRawMock.mockReset();
  // Clear the module-level cue cache between tests
  vi.resetModules();
});

describe("TrickplayPreview", () => {
  it("renders nothing when visible=false", () => {
    const { container } = render(
      <TrickplayPreview mediaId="m1" currentTime={5} visible={false} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders nothing when no cues loaded yet", async () => {
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve("") });
    const { container } = render(
      <TrickplayPreview mediaId="m-empty" currentTime={5} visible={true} />,
    );
    expect(container.innerHTML).toBe("");
    // The VTT fetch settles after the assertion; flush it inside act so the
    // resulting (still empty) state update is covered.
    await act(async () => {});
    expect(container.innerHTML).toBe("");
  });

  it("fetches VTT and renders matching cue", async () => {
    eveyFetchRawMock.mockResolvedValue({
      text: () => Promise.resolve(VTT_SAMPLE),
    });

    const { findByTestId } = render(
      <TrickplayPreview mediaId="m-vtt" currentTime={5} visible={true} />,
    );

    const img = await findByTestId("trickplay-img");
    expect(img.getAttribute("src")).toContain("tile0.jpg");
  });

  it("selects correct cue for currentTime in second range", async () => {
    eveyFetchRawMock.mockResolvedValue({
      text: () => Promise.resolve(VTT_SAMPLE),
    });

    const { findByTestId } = render(
      <TrickplayPreview mediaId="m-vtt2" currentTime={15} visible={true} />,
    );

    const img = await findByTestId("trickplay-img");
    // second cue still uses tile0.jpg but different xywh
    expect(img.getAttribute("src")).toContain("tile0.jpg");
  });
});

describe("parseVTT logic (via component)", () => {
  it("handles VTT with hour timestamps", async () => {
    const vtt = `WEBVTT

01:30:00.000 --> 01:30:10.000
/tile.jpg#xywh=0,0,160,90
`;
    eveyFetchRawMock.mockResolvedValue({ text: () => Promise.resolve(vtt) });

    const { findByTestId } = render(
      <TrickplayPreview mediaId="m-hour" currentTime={5400} visible={true} />,
    );

    const img = await findByTestId("trickplay-img");
    expect(img.getAttribute("src")).toContain("tile.jpg");
  });

  it("returns nothing for time outside any cue", async () => {
    eveyFetchRawMock.mockResolvedValue({
      text: () => Promise.resolve(VTT_SAMPLE),
    });

    const { container } = render(
      <TrickplayPreview mediaId="m-nocue" currentTime={999} visible={true} />,
    );

    // Wait a tick for fetch to complete - inside act, it updates state.
    await act(async () => {
      await new Promise((r) => setTimeout(r, 50));
    });
    // No cue matches time=999, so nothing renders (or empty)
    // The component returns null when no matching cue
    expect(container.querySelector("[data-testid='trickplay-img']")).toBeNull();
  });
});
