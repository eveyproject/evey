import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi } from "vitest";
import { EpisodesPanel } from "./PlayerEpisodesPanel";

vi.mock("@/components/ui/Select", () => ({
  Select: () => null,
}));

const defaultProps = {
  seasons: [{ id: "s1", title: "Season 1", indexNumber: 1 }],
  currentSeasonId: "s1",
  episodes: [
    { id: "e1", title: "Pilot", indexNumber: 1 },
    { id: "e2", title: "Second", indexNumber: 2 },
  ],
  currentEpisodeId: "e1",
  onSeasonChange: vi.fn(),
  onEpisodeChange: vi.fn(),
  open: false,
  onToggle: vi.fn(),
};

describe("EpisodesPanel", () => {
  it("renders episodes button", () => {
    render(<EpisodesPanel {...defaultProps} />);
    expect(screen.getByRole("button", { name: "Episodes" })).toBeInTheDocument();
  });

  it("shows episode list when open", () => {
    render(<EpisodesPanel {...defaultProps} open={true} />);
    expect(screen.getByText("Pilot")).toBeInTheDocument();
    expect(screen.getByText("Second")).toBeInTheDocument();
  });

  it("calls onEpisodeChange and onToggle when episode clicked", async () => {
    const user = userEvent.setup();
    const onEpisodeChange = vi.fn();
    const onToggle = vi.fn();
    render(<EpisodesPanel {...defaultProps} open={true} onEpisodeChange={onEpisodeChange} onToggle={onToggle} />);
    await user.click(screen.getByText("Second"));
    expect(onEpisodeChange).toHaveBeenCalledWith("e2");
    expect(onToggle).toHaveBeenCalled();
  });
});
