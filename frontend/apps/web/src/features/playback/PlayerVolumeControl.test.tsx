import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { PlayerVolumeControl } from "./PlayerVolumeControl";

const defaultProps = {
  volume: 0.8,
  muted: false,
  onToggleMute: vi.fn(),
  onVolumeChange: vi.fn(),
};

describe("PlayerVolumeControl", () => {
  it("renders mute button and volume slider", () => {
    render(<PlayerVolumeControl {...defaultProps} />);
    expect(screen.getByRole("button", { name: "Mute" })).toBeInTheDocument();
    expect(screen.getByRole("slider", { name: "Volume" })).toBeInTheDocument();
  });

  it("shows unmute label when muted", () => {
    render(<PlayerVolumeControl {...defaultProps} muted={true} />);
    expect(screen.getByRole("button", { name: "Unmute" })).toBeInTheDocument();
  });

  it("calls onToggleMute when mute button clicked", async () => {
    const user = userEvent.setup();
    const onToggleMute = vi.fn();
    render(<PlayerVolumeControl {...defaultProps} onToggleMute={onToggleMute} />);
    await user.click(screen.getByRole("button", { name: "Mute" }));
    expect(onToggleMute).toHaveBeenCalled();
  });
});
