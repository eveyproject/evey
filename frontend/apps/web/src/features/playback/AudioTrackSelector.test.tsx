import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PlayerControls } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";
import { formatAudioTrackLabel, formatChannelLayout, type AudioStream } from "./AudioTrackSelector";

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
  { index: 0, trackIndex: 0, language: "eng", title: "English", channels: 6 },
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

  it("renders available audio tracks in audio submenu, with the channel layout when known", async () => {
    const user = userEvent.setup();
    renderControls();

    await user.click(screen.getByRole("button", { name: "Settings" }));
    await user.click(screen.getByText("Audio"));

    const buttons = screen.getAllByRole("button");
    const labels = buttons.map((b) => b.textContent);
    // 6 channels reads as 5.1; the track with no channel count stays bare.
    expect(labels).toContain("English - 5.1");
    expect(labels).toContain("French");
  });

  describe("formatChannelLayout", () => {
    it("maps the common counts to speaker-layout notation", () => {
      expect(formatChannelLayout(1)).toBe("1.0");
      expect(formatChannelLayout(2)).toBe("2.0");
      expect(formatChannelLayout(6)).toBe("5.1");
      expect(formatChannelLayout(8)).toBe("7.1");
    });

    it("falls back to a raw count rather than guessing an unknown layout", () => {
      expect(formatChannelLayout(3)).toBe("3ch");
      expect(formatChannelLayout(7)).toBe("7ch");
    });

    it("renders nothing when the count is missing or nonsensical", () => {
      expect(formatChannelLayout(undefined)).toBe("");
      expect(formatChannelLayout(0)).toBe("");
      expect(formatChannelLayout(-2)).toBe("");
    });
  });

  describe("formatAudioTrackLabel", () => {
    it("prefers the title, then the language, then the caller's fallback", () => {
      expect(formatAudioTrackLabel({ index: 0, trackIndex: 0, title: "Commentary", language: "eng" }, "Track 1"))
        .toBe("Commentary");
      expect(formatAudioTrackLabel({ index: 0, trackIndex: 0, language: "eng" }, "Track 1")).toBe("eng");
      expect(formatAudioTrackLabel({ index: 0, trackIndex: 0 }, "Track 1")).toBe("Track 1");
    });

    it("appends the layout to whichever name was picked", () => {
      expect(formatAudioTrackLabel({ index: 0, trackIndex: 0, language: "fra", channels: 8 }, "Track 1"))
        .toBe("fra - 7.1");
      expect(formatAudioTrackLabel({ index: 0, trackIndex: 0, channels: 2 }, "Track 1")).toBe("Track 1 - 2.0");
    });
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
