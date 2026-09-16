import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children }: any) => React.createElement("div", null, children),
  Text: ({ children }: any) => React.createElement("span", null, children),
  Modal: ({ children }: any) => React.createElement("div", null, children),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  ActivityIndicator: () => React.createElement("span", { "data-testid": "spinner" }),
  Pressable: ({ children, onPress, disabled }: any) => React.createElement("button", { onClick: onPress, disabled }, children),
  StyleSheet: { create: (s: any) => s, absoluteFillObject: {}, hairlineWidth: 1 },
}));
const player = vi.hoisted(() => ({
  replaceAsync: vi.fn(), play: vi.fn(), pause: vi.fn(), playbackRate: 1, currentTime: 5,
}));
vi.mock("expo-video", () => ({ useVideoPlayer: () => player, VideoView: () => React.createElement("div", { "data-testid": "video" }) }));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (k: string, d?: string | Record<string, unknown>, o?: Record<string, unknown>) => {
      const opts = typeof d === "object" ? d : o;
      if (opts && Object.keys(opts).length) return `${k}:${Object.values(opts).join(",")}`;
      return typeof d === "string" ? d : k;
    },
  }),
}));
const applyShift = vi.hoisted(() => ({ mutate: vi.fn(), isPending: false, isError: false, isSuccess: false, error: null }));
const media = vi.hoisted(() => ({ data: undefined as unknown }));
vi.mock("@evey/api", () => ({
  apiErrorMessage: (e: Error) => e.message,
  useAdminMediaServiceApplyAudioShift: () => applyShift,
  useLibraryServiceGetMediaItem: () => media,
}));
vi.mock("@evey/store", () => ({ useAuthStore: (sel: (s: { accessToken: string }) => unknown) => sel({ accessToken: "tok" }) }));
vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({ colors: { text: "#fff", textMuted: "#888", border: "#333", accent: "#f00", onAccent: "#000", surface: "#111", danger: "#f33", warning: "#fa0", success: "#0f0" }, radii: { sm: 4, md: 8, lg: 12 } }),
}));
vi.mock("../../../hooks/useServerUrl", () => ({ useServerUrl: () => "https://s" }));

import { SyncPreviewModal } from "./SyncPreviewModal";

const item = { mediaId: "m 1", audioTracksLagSec: 0, audioDelaySec: 0.5, audioShortfallSec: 0 } as any;

beforeEach(() => {
  vi.clearAllMocks();
  player.replaceAsync.mockResolvedValue(undefined);
  player.playbackRate = 1;
  player.currentTime = 5;
  media.data = {
    durationMs: "1200000",
    streams: [
      { type: "MEDIA_STREAM_TYPE_VIDEO", frameRate: "50" },
      { type: "MEDIA_STREAM_TYPE_AUDIO", language: "fre", codec: "eac3", channels: 6 },
      { type: "MEDIA_STREAM_TYPE_AUDIO", language: "eng", codec: "aac", channels: 2 },
    ],
  };
});

const btn = (label: string) => screen.getByText(label).closest("button") as HTMLButtonElement;
// "-1"/"+1" exist in up to three rows (frame step once a clip is shown, then
// moment, then offset): pick by row order.
const nth = (label: string, i: number) => screen.getAllByText(label)[i].closest("button") as HTMLButtonElement;

// The clip is streamed to the native player with the admin bearer as a
// header, at the chosen moment/offset/track; a changed setting marks the
// shown clip stale; a frame step moves by one frame of the file's own rate;
// applying sends the 1-based track and is impossible at offset 0.
describe("SyncPreviewModal", () => {
  it("renders the clip with the bearer header and the chosen parameters, then flags a stale clip", async () => {
    render(<SyncPreviewModal item={item} title="Some Show" onClose={() => {}} />);
    expect(screen.getByText("Set a moment and offset, then Render.")).toBeTruthy();
    fireEvent.click(btn("+0.1"));
    fireEvent.click(btn("+0.05"));
    fireEvent.click(btn("+60"));
    fireEvent.click(btn("2 - eng · AAC · 2ch"));
    fireEvent.click(btn("admin.syncPreview.seconds:45"));

    fireEvent.click(btn("Render clip"));
    await waitFor(() => expect(player.replaceAsync).toHaveBeenCalledTimes(1));
    expect(player.replaceAsync).toHaveBeenCalledWith({
      uri: "https://s/api/v1/admin/media/m%201/sync-preview?at=660&len=45&track=1&shift=0.15",
      headers: { Authorization: "Bearer tok" },
    });
    await waitFor(() => expect(player.play).toHaveBeenCalledTimes(1));
    expect(screen.queryByText(/admin\.syncPreview\.stale/)).toBeNull();

    fireEvent.click(nth("-1", 1));
    expect(screen.getByText("admin.syncPreview.stale:+0.15,11:00")).toBeTruthy();
  });

  it("steps by one frame of the file's rate, changes speed, and starts from a measured offset", async () => {
    render(<SyncPreviewModal item={item} title="x" onClose={() => {}} />);
    fireEvent.click(btn("+0.5 s"));
    expect(screen.getByText("admin.syncPreview.apply:+0.5")).toBeTruthy();
    fireEvent.click(btn("Render clip"));
    await waitFor(() => expect(player.play).toHaveBeenCalled());

    fireEvent.click(btn("0.5x"));
    expect(player.playbackRate).toBe(0.5);
    fireEvent.click(nth("+1", 0));
    expect(player.pause).toHaveBeenCalled();
    expect(player.currentTime).toBeCloseTo(5.02, 5);
  });

  it("applies the 1-based track only with a non-zero offset, and reports a render failure", async () => {
    render(<SyncPreviewModal item={item} title="x" onClose={() => {}} />);
    const apply = btn("admin.syncPreview.apply:0");
    expect(apply.disabled).toBe(true);
    fireEvent.click(nth("+1", 1));
    fireEvent.click(btn("2 - eng · AAC · 2ch"));
    fireEvent.click(btn("admin.syncPreview.apply:+1"));
    expect(applyShift.mutate).toHaveBeenCalledWith({ mediaId: "m 1", data: { track: "2", shiftSec: 1 } });

    player.replaceAsync.mockRejectedValueOnce(new Error("no clip"));
    fireEvent.click(btn("Render clip"));
    await waitFor(() => expect(screen.getByText("no clip")).toBeTruthy());
    expect(player.play).not.toHaveBeenCalled();
  });
});
