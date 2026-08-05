import React from "react";
import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { MediaDetailFileInfo } from "./MediaDetailFileInfo";
import type { V1MediaItem } from "@evey/api";

vi.mock("./StreamList", () => ({
  StreamList: ({ streams }: { streams: unknown[] }) => (
    <div data-testid="stream-list">{streams.length} streams</div>
  ),
}));

// MediaTechSummary issues a GetStreamInfo RPC (the playback-difficulty badge is
// decided server-side, per device). Stub it here so this file keeps testing the
// disclosure alone; the summary has its own test.
vi.mock("./MediaTechSummary", () => ({
  MediaTechSummary: () => <div data-testid="media-tech-summary" />,
}));

function makeItem(overrides: Partial<V1MediaItem> = {}): V1MediaItem {
  return {
    id: "m-1",
    title: "Test Movie",
    filePath: "/media/movie.mkv",
    fileSize: "1073741824",
    containerFormat: "mkv",
    bitrate: "5000000",
    durationMs: "7200000",
    streams: [{ codec: "h264" }],
    ...overrides,
  } as V1MediaItem;
}

describe("MediaDetailFileInfo", () => {
  it("returns null when no file info available", () => {
    const { container } = render(
      <MediaDetailFileInfo item={{ id: "m-1" } as V1MediaItem} />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("returns null for a container (series/season) where the API emits proto zero values", () => {
    // EmitUnpopulated serializes int64 zeros as the STRING "0"; a naive
    // truthiness check would render the disclosure with a useless "0 B".
    const { container } = render(
      <MediaDetailFileInfo
        item={{ id: "season-1", filePath: "", fileSize: "0", containerFormat: "", durationMs: "0", bitrate: "0", streams: [] } as unknown as V1MediaItem}
      />,
    );
    expect(container.innerHTML).toBe("");
  });

  it("renders toggle button with file info label", () => {
    render(<MediaDetailFileInfo item={makeItem()} />);
    expect(screen.getByText("File Info")).toBeInTheDocument();
  });

  it("shows file details when expanded", async () => {
    const user = userEvent.setup();
    render(<MediaDetailFileInfo item={makeItem()} />);

    await user.click(screen.getByText("File Info"));

    expect(screen.getByText("/media/movie.mkv")).toBeInTheDocument();
    expect(screen.getByText("1.0 GB")).toBeInTheDocument();
    expect(screen.getByText("MKV")).toBeInTheDocument();
    expect(screen.getByText("5000 kbps")).toBeInTheDocument();
    expect(screen.getByText("2h")).toBeInTheDocument();
  });

  it("renders stream list when expanded", async () => {
    const user = userEvent.setup();
    render(<MediaDetailFileInfo item={makeItem()} />);

    await user.click(screen.getByText("File Info"));
    expect(screen.getByTestId("stream-list")).toBeInTheDocument();
  });

  it("hides details by default", () => {
    render(<MediaDetailFileInfo item={makeItem()} />);
    expect(screen.queryByText("/media/movie.mkv")).not.toBeInTheDocument();
  });

  it("toggles open and closed", async () => {
    const user = userEvent.setup();
    render(<MediaDetailFileInfo item={makeItem()} />);

    await user.click(screen.getByText("File Info"));
    expect(screen.getByText("/media/movie.mkv")).toBeInTheDocument();

    await user.click(screen.getByText("File Info"));
    expect(screen.queryByText("/media/movie.mkv")).not.toBeInTheDocument();
  });

  it("renders without optional fields", async () => {
    const user = userEvent.setup();
    render(<MediaDetailFileInfo item={makeItem({ filePath: "/a.mp4", fileSize: undefined, containerFormat: undefined, bitrate: undefined, durationMs: undefined, streams: [] })} />);

    await user.click(screen.getByText("File Info"));
    expect(screen.getByText("/a.mp4")).toBeInTheDocument();
    expect(screen.queryByTestId("stream-list")).not.toBeInTheDocument();
  });
});
