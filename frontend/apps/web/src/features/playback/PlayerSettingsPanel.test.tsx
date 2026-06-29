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
