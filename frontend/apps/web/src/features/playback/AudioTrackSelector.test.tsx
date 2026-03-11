import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PlayerControls } from "./PlayerControls";
import type { AudioStream } from "./AudioTrackSelector";

vi.mock("@/components/ui/Select", () => ({
  Select: () => null,
}));

vi.mock("./TrickplayPreview", () => ({
  TrickplayPreview: () => null,
}));

function createVideoRef() {
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { value: true, writable: true });
  Object.defineProperty(video, "currentTime", { value: 0, writable: true });
  Object.defineProperty(video, "duration", { value: 120, writable: true });
  Object.defineProperty(video, "volume", { value: 1, writable: true });
  return { current: video };
}

const audioStreams: AudioStream[] = [
  { index: 0, trackIndex: 0, language: "eng", title: "English" },
  { index: 1, trackIndex: 1, language: "fra", title: "French" },
];

const defaultProps = {
  videoRef: createVideoRef(),
  audioStreams,
  subtitleStreams: [],
  currentAudioIndex: 0,
  onAudioChange: vi.fn(),
  currentQuality: 0,
  onQualityChange: vi.fn(),
  currentSubtitleTrackIndex: null,
  onSubtitleChange: vi.fn(),
};

describe("AudioTrackSelector", () => {
  it("renders available audio tracks in settings panel", async () => {
    const user = userEvent.setup();
    render(<PlayerControls {...defaultProps} />);

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Audio")).toBeInTheDocument();
    expect(screen.getByText("English")).toBeInTheDocument();
    expect(screen.getByText("French")).toBeInTheDocument();
  });

  it("calls onAudioChange with selected track index", async () => {
    const user = userEvent.setup();
    const onAudioChange = vi.fn();
    render(<PlayerControls {...defaultProps} onAudioChange={onAudioChange} />);

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("French"));

    expect(onAudioChange).toHaveBeenCalledWith(1);
  });
});
