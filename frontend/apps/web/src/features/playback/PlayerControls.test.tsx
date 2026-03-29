import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PlayerControls } from "./PlayerControls";
import { PlayerProvider } from "./PlayerContext";

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

const defaultCallbacks = {
  onAudioChange: vi.fn(),
  onQualityChange: vi.fn(),
  onSubtitleChange: vi.fn(),
};

function renderWithProvider(callbackOverrides = {}, providerOverrides = {}) {
  const videoRef = createVideoRef();
  return render(
    <PlayerProvider
      videoRef={videoRef}
      audioStreams={[]}
      subtitleStreams={[]}
      currentAudioIndex={0}
      currentQuality={0}
      currentSubtitleTrackIndex={null}
      {...providerOverrides}
    >
      <PlayerControls {...defaultCallbacks} {...callbackOverrides} />
    </PlayerProvider>,
  );
}

describe("PlayerControls", () => {
  it("renders play/pause button", () => {
    renderWithProvider();
    expect(screen.getByRole("button", { name: "Play" })).toBeInTheDocument();
  });

  it("renders volume slider", () => {
    renderWithProvider();
    expect(screen.getByRole("slider", { name: "Volume" })).toBeInTheDocument();
  });

  it("opens settings panel on settings button click", async () => {
    const user = userEvent.setup();
    renderWithProvider({ onSpeedChange: vi.fn() });
    const settingsBtn = screen.getByRole("button", { name: "Settings" });
    await user.click(settingsBtn);
    expect(screen.getByText("Quality")).toBeInTheDocument();
  });

  it("shows playback speed selector in settings submenu", async () => {
    const user = userEvent.setup();
    renderWithProvider({ onSpeedChange: vi.fn() }, { playbackSpeed: 1 });
    await user.click(screen.getByRole("button", { name: "Settings" }));
    expect(screen.getByText("Speed")).toBeInTheDocument();
    await user.click(screen.getByText("Speed"));
    expect(screen.getByText("0.5x")).toBeInTheDocument();
    expect(screen.getByText("2x")).toBeInTheDocument();
  });

  it("renders fullscreen button", () => {
    renderWithProvider();
    expect(screen.getByRole("button", { name: "Fullscreen" })).toBeInTheDocument();
  });
});
