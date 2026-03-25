import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { SettingsPanel } from "./PlayerSettingsPanel";

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
