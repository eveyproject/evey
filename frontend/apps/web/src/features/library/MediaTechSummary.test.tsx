import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import type { V1MediaItem } from "@evey/api";
import { MediaTechSummary } from "./MediaTechSummary";

const mockDifficulties = vi.fn();

// The summary asks the SAME bulk hook the lists use, with a single id, rather
// than GetStreamInfo - so the badge cannot diverge from what a list shows, and
// visiting a media page no longer triggers the keyframe on-demand disk probe.
vi.mock("@/hooks/usePlaybackDifficulties", () => ({
  usePlaybackDifficulties: (...args: unknown[]) => mockDifficulties(...args),
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
    mockDifficulties.mockReturnValue(new Map());
  });

  it("shows only what a viewer reads: size of picture, HDR, sound, weight", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.getByText("4K")).toBeInTheDocument();
    expect(screen.getByText("HDR10")).toBeInTheDocument();
    expect(screen.getByText("5.1")).toBeInTheDocument();
    expect(screen.getByText("1.0 GB")).toBeInTheDocument();
  });

  // Codec and container names were the bulk of the jargon and say nothing to
  // most people. They belong in the file-info disclosure, not on the headline.
  it("keeps codec and container names out of the visible line", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.queryByText(/HEVC/)).not.toBeInTheDocument();
    expect(screen.queryByText(/EAC3/)).not.toBeInTheDocument();
    expect(screen.queryByText(/MKV|MATROSKA/)).not.toBeInTheDocument();
  });

  it("renders no difficulty badge until the server has decided", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.queryByTestId("playback-difficulty")).not.toBeInTheDocument();
  });

  it("shows the verdict the server returned for this item", () => {
    mockDifficulties.mockReturnValue(
      new Map([["m-1", { level: "heavy", difficulty: "hard", reasons: [] }]]),
    );
    render(<MediaTechSummary item={makeItem()} />);
    expect(screen.getByText("Very heavy")).toBeInTheDocument();
  });

  it("explains the badge in one plain sentence, not in reason codes", () => {
    mockDifficulties.mockReturnValue(
      new Map([["m-1", { level: "heavy", difficulty: "hard", reasons: ["hdr_tonemapping_needed"] }]]),
    );
    render(<MediaTechSummary item={makeItem()} />);
    const badge = screen.getByTestId("playback-difficulty");
    expect(badge.getAttribute("title")).toContain("converted");
    expect(badge.getAttribute("title")).not.toContain("hdr_tonemapping_needed");
  });

  it("does not query for a container that has no file of its own", () => {
    render(<MediaTechSummary item={{ id: "s-1", fileSize: "0" } as V1MediaItem} />);
    expect(mockDifficulties).toHaveBeenCalledWith([]);
  });

  it("asks for exactly the one id it is showing", () => {
    render(<MediaTechSummary item={makeItem()} />);
    expect(mockDifficulties).toHaveBeenCalledWith(["m-1"]);
  });

  it("reads the channel count of the default audio track", () => {
    const item = makeItem({
      streams: [
        { index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", width: 1920, height: 1080 },
        { index: 1, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "aac", channels: 2, isDefault: true },
        { index: 2, type: "MEDIA_STREAM_TYPE_AUDIO", codec: "ac3", channels: 6 },
      ],
    } as Partial<V1MediaItem>);
    render(<MediaTechSummary item={item} />);
    expect(screen.getByText("2.0")).toBeInTheDocument();
    expect(screen.getByText("1080p")).toBeInTheDocument();
  });
});
