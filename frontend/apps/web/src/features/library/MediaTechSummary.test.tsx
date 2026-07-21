import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { V1MediaItem } from "@evey/api";
import { MediaTechSummary } from "./MediaTechSummary";

const mockStreamInfo = vi.fn();

vi.mock("@evey/api", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  usePlaybackServiceGetStreamInfo: (...args: unknown[]) => mockStreamInfo(...args),
}));

const mockWasm = vi.fn(() => false);

vi.mock("@/utils/codec", async (orig) => ({
  ...(await orig<Record<string, unknown>>()),
  getBrowserCapabilities: () => ({
    videoCodecs: ["h264"],
    audioCodecs: ["aac"],
    containers: ["mp4"],
    hdr: false,
  }),
  canWasmDecodeAC3: () => mockWasm(),
}));

function makeItem(overrides: Partial<V1MediaItem> = {}): V1MediaItem {
  return {
    id: "m-1",
    fileSize: "1073741824",
    containerFormat: "mkv",
    streams: [
      { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "hevc", width: 3840, height: 2160, colorTransfer: "smpte2084" },
      { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", channels: 6, isDefault: true },
    ],
    ...overrides,
  } as V1MediaItem;
}

describe("MediaTechSummary", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockStreamInfo.mockReturnValue({ data: undefined });
  });

  it("shows only what a viewer reads: size of picture, HDR, sound, weight", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.getByText("4K")).toBeInTheDocument();
    expect(screen.getByText("HDR10")).toBeInTheDocument();
    expect(screen.getByText("5.1")).toBeInTheDocument();
    expect(screen.getByText("1.0 GB")).toBeInTheDocument();
  });

  it("renders no difficulty badge until the server has decided", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.queryByTestId("playback-difficulty")).not.toBeInTheDocument();
  });

  it("reports a heavy transcode for 4K HDR that the device can't take", () => {
    mockStreamInfo.mockReturnValue({
      data: {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        audioDecision: "transcode",
        transcodeReasons: ["hdr_tonemapping_needed"],
      },
    });
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.getByText("Very heavy")).toBeInTheDocument();
  });

  it("reports direct play when the server asks for nothing", () => {
    mockStreamInfo.mockReturnValue({
      data: { playMethod: "PLAY_METHOD_DIRECT_PLAY", transcodeReasons: [] },
    });
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.getByText("Light")).toBeInTheDocument();
  });

  it("does not query for a container that has no file of its own", () => {
    render(<MediaTechSummary item={{ id: "s-1", fileSize: "0" } as V1MediaItem} />);
    const opts = mockStreamInfo.mock.calls[0]?.[2] as { query?: { enabled?: boolean } };
    expect(opts?.query?.enabled).toBe(false);
  });

  it("never sends screen dimensions as a decode capability", () => {
    render(<MediaTechSummary item={makeItem()} />);
    const params = mockStreamInfo.mock.calls[0]?.[1] as Record<string, unknown>;
    expect(params).not.toHaveProperty("maxWidth");
    expect(params).not.toHaveProperty("maxHeight");
    expect(params.supportsHdr).toBe(false);
  });

  it("does not bill AC-3 as a transcode when the browser decodes it in wasm", () => {
    mockWasm.mockReturnValue(true);
    mockStreamInfo.mockReturnValue({
      data: {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "copy",
        audioDecision: "transcode",
        transcodeReasons: ["audio_codec_unsupported"],
      },
    });
    render(<MediaTechSummary item={makeItem({ streams: [
      { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", width: 1920, height: 1080 },
      { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "eac3", channels: 6, isDefault: true },
    ] } as Partial<V1MediaItem>)} />);
    expect(screen.getByText("Light")).toBeInTheDocument();
  });

  // Codec and container names were the bulk of the jargon and say nothing to
  // most people. They belong in the file-info disclosure, not on the headline.
  it("keeps codec and container names out of the visible line", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.queryByText(/HEVC/)).not.toBeInTheDocument();
    expect(screen.queryByText(/EAC3/)).not.toBeInTheDocument();
    expect(screen.queryByText(/MKV|MATROSKA/)).not.toBeInTheDocument();
  });

  it("explains the badge in one plain sentence, not in reason codes", () => {
    mockStreamInfo.mockReturnValue({
      data: {
        playMethod: "PLAY_METHOD_TRANSCODE",
        videoDecision: "transcode",
        transcodeReasons: ["hdr_tonemapping_needed"],
      },
    });
    render(<MediaTechSummary item={makeItem()} />);
    const badge = screen.getByTestId("playback-difficulty");
    expect(badge).toHaveAttribute("title", expect.stringContaining("converted"));
    expect(badge.getAttribute("title")).not.toContain("hdr_tonemapping_needed");
  });

  it("counts the extra audio tracks", () => {
    const item = makeItem({
      streams: [
        { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", width: 1920, height: 1080 },
        { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", channels: 2, isDefault: true },
        { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3", channels: 6 },
      ],
    } as Partial<V1MediaItem>);
    render(<MediaTechSummary item={item} />);
    expect(screen.getByText("2.0")).toBeInTheDocument();
  });
});
