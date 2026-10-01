import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StreamList } from "./StreamList";

const audioStream = {
  index: 0,
  type: "MEDIA_STREAM_TYPE_AUDIO",
  codec: "AAC",
  language: "eng",
  title: "Stereo",
  isDefault: true,
  isForced: false,
  isExternal: false,
};
const subtitleStream = {
  index: 1,
  type: "MEDIA_STREAM_TYPE_SUBTITLE",
  codec: "SRT",
  language: "fre",
  title: "French",
  isDefault: false,
  isForced: true,
  isExternal: false,
};

describe("StreamList", () => {
  it("renders nothing when no streams", () => {
    const { container } = render(<StreamList streams={[]} />);
    expect(container.firstChild).toBeNull();
  });

  it("renders nothing when undefined", () => {
    const { container } = render(<StreamList />);
    expect(container.firstChild).toBeNull();
  });

  it("renders audio section", () => {
    render(<StreamList streams={[audioStream as never]} />);
    expect(screen.getByText("Audio")).toBeInTheDocument();
    expect(screen.getByText("AAC")).toBeInTheDocument();
    // The container code reads as a name ("eng" was printed raw).
    expect(screen.getByText("English")).toBeInTheDocument();
    expect(screen.getByText("Default")).toBeInTheDocument();
  });

  it("renders subtitle section", () => {
    render(<StreamList streams={[subtitleStream as never]} />);
    expect(screen.getByText("Subtitles")).toBeInTheDocument();
    expect(screen.getByText("French")).toBeInTheDocument();
    expect(screen.getByText("Forced")).toBeInTheDocument();
  });

  it("renders both sections when both stream types present", () => {
    render(<StreamList streams={[audioStream, subtitleStream] as never[]} />);
    expect(screen.getByText("Audio")).toBeInTheDocument();
    expect(screen.getByText("Subtitles")).toBeInTheDocument();
  });
});

describe("StreamList video row", () => {
  it("names a cropped scope frame by its tier, then its exact size", () => {
    render(<StreamList streams={[{ index: 0, type: "MEDIA_STREAM_TYPE_VIDEO", codec: "h264", width: 1920, height: 818 } as never]} />);
    expect(screen.getByText("1080p · 1920×818")).toBeInTheDocument();
  });
});
