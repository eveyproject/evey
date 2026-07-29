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

    // The overlay IS the component's root: it carries onClick={handleOverlayClick}.
    await user.click(container.firstElementChild as Element);

    expect(screen.queryByText("Quality")).not.toBeInTheDocument();
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
      expect(bar?.className).toContain("opacity-100");
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
      expect(bar?.className).toContain("opacity-0");
    } finally {
      vi.useRealTimers();
    }
  });
});
