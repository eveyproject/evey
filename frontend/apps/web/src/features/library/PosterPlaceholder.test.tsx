import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { PosterPlaceholder } from "./PosterPlaceholder";

vi.mock("lucide-react", () => ({
  Clapperboard: (p: any) => React.createElement("i", { "data-testid": "icon-movie", "data-size": p.size }),
  Tv: () => React.createElement("i", { "data-testid": "icon-tv" }),
  Film: () => React.createElement("i", { "data-testid": "icon-film" }),
}));

// A posterless card reads as designed: the kind icon (TV for anything
// episodic, clapperboard for a film, generic otherwise), the title or its
// initial, and an accessible name even without a title.
describe("PosterPlaceholder", () => {
  it("picks the icon by media kind", () => {
    const { rerender } = render(<PosterPlaceholder type="MEDIA_ITEM_TYPE_MOVIE" title="Heat" iconSize={44} />);
    expect(screen.getByTestId("icon-movie").getAttribute("data-size")).toBe("44");
    rerender(<PosterPlaceholder type="MEDIA_ITEM_TYPE_EPISODE" title="E1" />);
    expect(screen.getByTestId("icon-tv")).toBeInTheDocument();
    rerender(<PosterPlaceholder type="MEDIA_ITEM_TYPE_SEASON" title="S1" />);
    expect(screen.getByTestId("icon-tv")).toBeInTheDocument();
    rerender(<PosterPlaceholder title="Track" />);
    expect(screen.getByTestId("icon-film")).toBeInTheDocument();
  });

  it("labels the image with the title, shows it or hides it on request", () => {
    const { rerender } = render(<PosterPlaceholder title="Heat" />);
    expect(screen.getByRole("img", { name: "Heat" })).toHaveTextContent("Heat");
    rerender(<PosterPlaceholder title="Heat" showTitle={false} />);
    expect(screen.getByRole("img", { name: "Heat" })).toHaveTextContent("");
  });

  it("without a title: generic label and a question mark", () => {
    render(<PosterPlaceholder />);
    expect(screen.getByRole("img", { name: "Media poster" })).toHaveTextContent("?");
  });
});
