import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { SettingsPanel, assToVtt, srtToVtt } from "./PlayerSettingsPanel";

const defaultProps = {
  audioStreams: [],
  subtitleStreams: [],
  currentAudioIndex: 0,
  onAudioChange: vi.fn(),
  currentQuality: 0,
  onQualityChange: vi.fn(),
  currentSubtitleTrackIndex: null,
  onSubtitleChange: vi.fn(),
  open: false,
  onToggle: vi.fn(),
};

describe("SettingsPanel", () => {
  it("renders settings button", () => {
    render(<SettingsPanel {...defaultProps} />);
    expect(screen.getByRole("button", { name: "Settings" })).toBeInTheDocument();
  });

  it("shows quality options when open", () => {
    render(<SettingsPanel {...defaultProps} open={true} />);
    expect(screen.getByText("Quality")).toBeInTheDocument();
  });

  it("shows speed submenu when onSpeedChange provided", async () => {
    const user = userEvent.setup();
    render(<SettingsPanel {...defaultProps} open={true} onSpeedChange={vi.fn()} />);
    expect(screen.getByText("Speed")).toBeInTheDocument();
    await user.click(screen.getByText("Speed"));
    expect(screen.getByText("0.5x")).toBeInTheDocument();
    expect(screen.getByText("2x")).toBeInTheDocument();
  });

  it("shows audio submenu when audio streams exist", async () => {
    const user = userEvent.setup();
    const audioStreams = [
      { index: 0, trackIndex: 0, language: "eng", title: "English" },
      { index: 1, trackIndex: 1, language: "fra", title: "French" },
    ];
    render(<SettingsPanel {...defaultProps} audioStreams={audioStreams} open={true} />);
    expect(screen.getByText("Audio")).toBeInTheDocument();
    await user.click(screen.getByText("Audio"));
    expect(screen.getByText("English")).toBeInTheDocument();
    expect(screen.getByText("French")).toBeInTheDocument();
  });

  it("calls onQualityChange when quality selected", async () => {
    const user = userEvent.setup();
    const onQualityChange = vi.fn();
    const onToggle = vi.fn();
    render(<SettingsPanel {...defaultProps} open={true} onQualityChange={onQualityChange} onToggle={onToggle} />);
    await user.click(screen.getByText("720p"));
    expect(onQualityChange).toHaveBeenCalledWith(720);
  });
});

describe("SettingsPanel sleep timer submenu", () => {
  // Assertions use data-testid rather than translated copy: the sleep timer
  // i18n keys are brand new and the test harness's translation fixture is a
  // point-in-time snapshot of the backend catalogue, so relying on the
  // rendered English string would be a false negative until that snapshot
  // is refreshed.
  it("shows sleep timer submenu with presets and end-of-episode option", async () => {
    const user = userEvent.setup();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        onSleepTimerMinutes={vi.fn()}
        onSleepTimerEndOfEpisode={vi.fn()}
      />,
    );
    expect(screen.getByTestId("sleep-timer-trigger")).toBeInTheDocument();
    await user.click(screen.getByTestId("sleep-timer-trigger"));
    expect(screen.getByTestId("sleep-timer-15")).toBeInTheDocument();
    expect(screen.getByTestId("sleep-timer-90")).toBeInTheDocument();
    expect(screen.getByTestId("sleep-timer-end-of-episode")).toBeInTheDocument();
  });

  it("calls onSleepTimerMinutes when a preset is picked", async () => {
    const user = userEvent.setup();
    const onSleepTimerMinutes = vi.fn();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        onSleepTimerMinutes={onSleepTimerMinutes}
      />,
    );
    await user.click(screen.getByTestId("sleep-timer-trigger"));
    await user.click(screen.getByTestId("sleep-timer-30"));
    expect(onSleepTimerMinutes).toHaveBeenCalledWith(30);
  });

  it("calls onSleepTimerEndOfEpisode when that option is picked", async () => {
    const user = userEvent.setup();
    const onSleepTimerEndOfEpisode = vi.fn();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        onSleepTimerMinutes={vi.fn()}
        onSleepTimerEndOfEpisode={onSleepTimerEndOfEpisode}
      />,
    );
    await user.click(screen.getByTestId("sleep-timer-trigger"));
    await user.click(screen.getByTestId("sleep-timer-end-of-episode"));
    expect(onSleepTimerEndOfEpisode).toHaveBeenCalledTimes(1);
  });

  it("shows the cancel row only when a timer is active, and calls onSleepTimerCancel", async () => {
    const user = userEvent.setup();
    const onSleepTimerCancel = vi.fn();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        onSleepTimerMinutes={vi.fn()}
        onSleepTimerCancel={onSleepTimerCancel}
        sleepTimerState={{ kind: "minutes", minutesLeft: 12 }}
      />,
    );
    await user.click(screen.getByTestId("sleep-timer-trigger"));
    expect(screen.getByTestId("sleep-timer-cancel")).toBeInTheDocument();
    await user.click(screen.getByTestId("sleep-timer-cancel"));
    expect(onSleepTimerCancel).toHaveBeenCalledTimes(1);
  });

  it("does not show the cancel row when no timer is active", async () => {
    const user = userEvent.setup();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        onSleepTimerMinutes={vi.fn()}
        onSleepTimerCancel={vi.fn()}
        sleepTimerState={null}
      />,
    );
    await user.click(screen.getByTestId("sleep-timer-trigger"));
    expect(screen.queryByTestId("sleep-timer-cancel")).not.toBeInTheDocument();
  });

  it("does not render the sleep timer entry without a handler", () => {
    render(<SettingsPanel {...defaultProps} open={true} />);
    expect(screen.queryByTestId("sleep-timer-trigger")).not.toBeInTheDocument();
  });
});

describe("srtToVtt", () => {
  it("converts SRT comma timestamps to VTT periods under a WEBVTT header", () => {
    const srt = "1\n00:00:01,000 --> 00:00:03,500\nHello\n";
    const vtt = srtToVtt(srt);
    expect(vtt).toMatch(/^WEBVTT/);
    expect(vtt).toContain("00:00:01.000 --> 00:00:03.500");
    expect(vtt).toContain("Hello");
  });
});

describe("assToVtt", () => {
  const ass = [
    "[Script Info]",
    "Title: Sample",
    "[Events]",
    "Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text",
    "Dialogue: 0,0:00:01.00,0:00:03.50,Default,,0,0,0,,{\\i1}Hello{\\i0} world",
    "Dialogue: 0,0:00:04.00,0:00:05.00,Default,,0,0,0,,Line one\\NLine two, with comma",
    "Comment: 0,0:00:09.00,0:00:10.00,Default,,0,0,0,,ignored",
  ].join("\n");

  it("emits a WEBVTT header and one cue per Dialogue line (Comment ignored)", () => {
    const vtt = assToVtt(ass);
    expect(vtt).toMatch(/^WEBVTT/);
    expect((vtt.match(/-->/g) ?? []).length).toBe(2);
    expect(vtt).not.toContain("ignored");
  });

  it("converts centisecond timestamps to VTT milliseconds", () => {
    expect(assToVtt(ass)).toContain("00:00:01.000 --> 00:00:03.500");
  });

  it("strips ASS override tags and converts \\N to newlines, keeping commas in text", () => {
    const vtt = assToVtt(ass);
    expect(vtt).toContain("Hello world");
    expect(vtt).not.toContain("\\i1");
    expect(vtt).toContain("Line one\nLine two, with comma");
  });
});

describe("OffsetStepper (audio/subtitle resync rows)", () => {
  it("steps the audio offset by 100ms, clamps at 5s, and resets to zero", async () => {
    const user = userEvent.setup();
    const onAudioOffsetChange = vi.fn();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        audioStreams={[{ index: 0, codec: "aac", title: "Main" }]}
        audioOffsetMs={4950}
        onAudioOffsetChange={onAudioOffsetChange}
      />,
    );
    await user.click(screen.getByText("Audio"));
    await user.click(screen.getByTestId("audio-offset-plus"));
    expect(onAudioOffsetChange).toHaveBeenLastCalledWith(5000);
    await user.click(screen.getByTestId("audio-offset-minus"));
    expect(onAudioOffsetChange).toHaveBeenLastCalledWith(4850);
    await user.click(screen.getByTestId("audio-offset-reset"));
    expect(onAudioOffsetChange).toHaveBeenLastCalledWith(0);
  });

  it("disables the audio resync controls outside transcoded playback", async () => {
    const user = userEvent.setup();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        audioStreams={[{ index: 0, codec: "aac", title: "Main" }]}
        audioOffsetMs={0}
        onAudioOffsetChange={vi.fn()}
        audioOffsetAvailable={false}
      />,
    );
    await user.click(screen.getByText("Audio"));
    expect(screen.getByTestId("audio-offset-plus")).toBeDisabled();
    expect(screen.getByTestId("audio-offset-minus")).toBeDisabled();
  });

  it("shows the subtitle resync stepper in the subtitle style submenu", async () => {
    const user = userEvent.setup();
    const onSubtitleOffsetChange = vi.fn();
    render(
      <SettingsPanel
        {...defaultProps}
        open={true}
        onSubtitleStyleChange={vi.fn()}
        subtitleOffsetMs={0}
        onSubtitleOffsetChange={onSubtitleOffsetChange}
      />,
    );
    await user.click(screen.getByText("Subtitle Style"));
    await user.click(screen.getByTestId("subtitle-offset-minus"));
    expect(onSubtitleOffsetChange).toHaveBeenLastCalledWith(-100);
  });
});

describe("SettingsPanel auto-play next episode", () => {
  it("shows the switch only when the caller can persist the choice", () => {
    render(<SettingsPanel {...defaultProps} open={true} />);
    expect(screen.queryByRole("switch", { name: /Auto-play next episode/ })).not.toBeInTheDocument();
  });

  it("flips the choice on click and reads the current state", async () => {
    const user = userEvent.setup();
    const onAutoplayNextChange = vi.fn();
    render(<SettingsPanel {...defaultProps} open={true} autoplayNext={false} onAutoplayNextChange={onAutoplayNextChange} />);
    const row = screen.getByRole("switch", { name: /Auto-play next episode/ });
    expect(row).toHaveAttribute("aria-checked", "false");
    await user.click(row);
    expect(onAutoplayNextChange).toHaveBeenCalledWith(true);
  });
});
