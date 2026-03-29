import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PlayerControls } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";
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

function renderControls(overrides: Record<string, unknown> = {}) {
  const videoRef = createVideoRef();
  const { onAudioChange, onQualityChange, onSubtitleChange, ...providerOverrides } = overrides;
  return render(
    <PlayerProvider
      videoRef={videoRef}
      audioStreams={audioStreams}
      subtitleStreams={[]}
      currentAudioIndex={0}
      currentQuality={0}
      currentSubtitleTrackIndex={null}
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

describe("AudioTrackSelector", () => {
  it("renders audio submenu entry in settings panel", async () => {
    const user = userEvent.setup();
    renderControls();

    await user.click(screen.getByRole("button", { name: "Settings" }));

    expect(screen.getByText("Audio")).toBeInTheDocument();
  });

  it("renders available audio tracks in audio submenu", async () => {
    const user = userEvent.setup();
    renderControls();

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("Audio"));

    const buttons = screen.getAllByRole("button");
    const labels = buttons.map((b) => b.textContent);
    expect(labels).toContain("English");
    expect(labels).toContain("French");
  });

  it("calls onAudioChange with selected track index", async () => {
    const user = userEvent.setup();
    const onAudioChange = vi.fn();
    renderControls({ onAudioChange });

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("Audio"));
    await user.click(screen.getByText("French"));

    expect(onAudioChange).toHaveBeenCalledWith(1);
  });
});
