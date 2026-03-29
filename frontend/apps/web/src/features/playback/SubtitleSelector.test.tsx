import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PlayerControls } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";
import type { SubtitleStream } from "./SubtitleSelector";

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

const subtitleStreams: SubtitleStream[] = [
  { index: 0, trackIndex: 0, language: "eng", title: "English" },
  { index: 1, trackIndex: 1, language: "fra", title: "French" },
];

function renderControls(overrides: Record<string, unknown> = {}) {
  const videoRef = createVideoRef();
  const { currentSubtitleTrackIndex, onSubtitleChange, onAudioChange, onQualityChange, ...providerOverrides } = overrides;
  return render(
    <PlayerProvider
      videoRef={videoRef}
      audioStreams={[]}
      subtitleStreams={subtitleStreams}
      currentAudioIndex={0}
      currentQuality={0}
      currentSubtitleTrackIndex={(currentSubtitleTrackIndex as number | null) ?? null}
      {...providerOverrides}
    >
      <PlayerControls
        onAudioChange={(onAudioChange as (i: number) => void) ?? vi.fn()}
        onQualityChange={(onQualityChange as (q: number) => void) ?? vi.fn()}
        onSubtitleChange={(onSubtitleChange as (t: number | null) => void) ?? vi.fn()}
      />
    </PlayerProvider>,
  );
}

describe("SubtitleSelector", () => {
  it("renders subtitle submenu entry in settings panel", async () => {
    const user = userEvent.setup();
    renderControls();

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Subtitles")).toBeInTheDocument();
  });

  it("renders available subtitle tracks in subtitles submenu", async () => {
    const user = userEvent.setup();
    renderControls();

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("Subtitles"));

    expect(screen.getByText("Off")).toBeInTheDocument();
    expect(screen.getByText("English")).toBeInTheDocument();
    expect(screen.getByText("French")).toBeInTheDocument();
  });

  it("calls onSubtitleChange with trackIndex when subtitle selected", async () => {
    const user = userEvent.setup();
    const onSubtitleChange = vi.fn();
    renderControls({ onSubtitleChange });

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("Subtitles"));
    await user.click(screen.getByText("French"));

    expect(onSubtitleChange).toHaveBeenCalledWith(1);
  });

  it("calls onSubtitleChange with null when Off selected", async () => {
    const user = userEvent.setup();
    const onSubtitleChange = vi.fn();
    renderControls({ currentSubtitleTrackIndex: 0, onSubtitleChange });

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("Subtitles"));
    await user.click(screen.getByText("Off"));

    expect(onSubtitleChange).toHaveBeenCalledWith(null);
  });
});
