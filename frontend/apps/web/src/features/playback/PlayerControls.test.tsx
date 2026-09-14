import { render, screen, act, fireEvent } from "@testing-library/react";
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

  // Closing an open panel by clicking away used to be each panel's own job
  // (PlayOnPicker carried a document listener). It now belongs to this
  // component's shared openPanel state, so the behaviour has to be pinned
  // HERE or it is covered nowhere - the panels' own tests can no longer see
  // it. A click on the overlay must close the panel and NOT fall through to
  // the click-to-fullscreen / play-pause handling underneath.
  it("closes an open panel when the overlay is clicked, without toggling playback", async () => {
    const user = userEvent.setup();
    const { container } = renderWithProvider({ onSpeedChange: vi.fn() });

    await user.click(screen.getByRole("button", { name: "Settings" }));
    expect(screen.getByText("Quality")).toBeInTheDocument();

    // The bare video surface is the dedicated element under the bars.
    await user.click(container.querySelector("[data-player-surface]") as Element);

    expect(screen.queryByText("Quality")).not.toBeInTheDocument();
  });
});

// Reported live: opening the settings menu started and stopped the sound. The
// bottom bar and its panels must be sealed off from the surface tap that
// toggles play/pause (single click) and fullscreen (double click) - a click
// on a control is never a click on the video.
describe("PlayerControls surface isolation", () => {
  function playingVideoRef() {
    const ref = createVideoRef();
    Object.defineProperty(ref.current, "paused", { value: false, writable: true });
    ref.current.play = vi.fn().mockResolvedValue(undefined);
    ref.current.pause = vi.fn();
    return ref;
  }

  function renderPlaying() {
    const videoRef = playingVideoRef();
    const utils = render(
      <PlayerProvider
        videoRef={videoRef}
        audioStreams={[]}
        subtitleStreams={[]}
        currentAudioIndex={0}
        currentQuality={0}
        currentSubtitleTrackIndex={null}
        playbackSpeed={1}
      >
        <PlayerControls {...defaultCallbacks} onSpeedChange={vi.fn()} />
      </PlayerProvider>,
    );
    return { ...utils, video: videoRef.current };
  }

  it("a click on the settings gear, then inside its menu, never toggles playback", () => {
    vi.useFakeTimers();
    try {
      const { video } = renderPlaying();
      fireEvent.click(screen.getByRole("button", { name: "Settings" }));
      expect(screen.getByText("Quality")).toBeInTheDocument();
      fireEvent.click(screen.getByText("Speed"));
      fireEvent.click(screen.getByText("Speed"));
      // Past the single-click delay: a leaked click would have fired by now.
      act(() => { vi.advanceTimersByTime(500); });
      expect(video.pause).not.toHaveBeenCalled();
      expect(video.play).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("a click anywhere on the bottom bar (not on a button) never toggles playback", () => {
    vi.useFakeTimers();
    try {
      const { video } = renderPlaying();
      const bar = screen.getByRole("slider", { name: "Volume" }).closest("[data-controls]") as HTMLElement;
      fireEvent.click(bar);
      fireEvent.click(bar);
      act(() => { vi.advanceTimersByTime(500); });
      expect(video.pause).not.toHaveBeenCalled();
      expect(video.play).not.toHaveBeenCalled();
    } finally {
      vi.useRealTimers();
    }
  });

  it("a single click on the bare surface still toggles playback", () => {
    vi.useFakeTimers();
    try {
      const { container, video } = renderPlaying();
      fireEvent.click(container.querySelector("[data-player-surface]") as Element);
      act(() => { vi.advanceTimersByTime(250); });
      expect(video.pause).toHaveBeenCalledTimes(1);
    } finally {
      vi.useRealTimers();
    }
  });
});

describe("PlayerControls auto-hide vs keyboard focus", () => {
  // Hiding the bar under a keyboard user's focus breaks WCAG focus visibility
  // (observed live: the volume slider vanished mid-arrow-keys and the presses
  // went dead). Keyboard focus inside the overlay must pin the controls.
  it("does not hide while a control inside holds keyboard focus", () => {
    vi.useFakeTimers();
    try {
      renderWithProvider();
      const slider = screen.getByRole("slider", { name: "Volume" });
      slider.focus();
      expect(document.activeElement).toBe(slider);
      // The pin is keyed on keyboard modality: an arrow press on the slider
      // is exactly the interaction that used to lose the bar mid-adjustment.
      fireEvent.keyDown(slider, { key: "ArrowRight" });

      act(() => { vi.advanceTimersByTime(3500); });

      const bar = slider.closest("[data-controls]");
      expect(bar?.className).toContain("fade-shown");
    } finally {
      vi.useRealTimers();
    }
  });

  it("still hides after the timeout when nothing inside is focused", () => {
    vi.useFakeTimers();
    try {
      renderWithProvider();
      const slider = screen.getByRole("slider", { name: "Volume" });
      act(() => { vi.advanceTimersByTime(3500); });
      const bar = slider.closest("[data-controls]");
      expect(bar?.className).toContain("fade-hidden");
    } finally {
      vi.useRealTimers();
    }
  });
});
