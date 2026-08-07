import { describe, it, expect } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import { RequestPoster } from "./RequestPoster";

describe("RequestPoster", () => {
  it("renders the poster when there is a url", () => {
    render(<RequestPoster posterUrl="https://example.test/p.jpg" title="Some Movie" mediaType="movie" />);
    expect(screen.getByRole("img")).toHaveAttribute("src", "https://example.test/p.jpg");
  });

  it("falls back when there is no url at all", () => {
    render(<RequestPoster mediaType="movie" />);
    expect(screen.getByTestId("request-poster-fallback")).toBeInTheDocument();
  });

  // The real-world case this component exists for: the MusicBrainz provider
  // builds a coverartarchive URL from the release-group id without checking the
  // artwork exists, so a request stored months ago can point at a 404/500. The
  // admin moderation queue used to render that as a broken image.
  it("falls back when the poster url fails to load", () => {
    render(
      <RequestPoster
        posterUrl="https://coverartarchive.org/release-group/nope/front-250"
        title="Some Album"
        mediaType="music"
      />,
    );
    fireEvent.error(screen.getByRole("img"));
    expect(screen.getByTestId("request-poster-fallback")).toBeInTheDocument();
    expect(screen.queryByRole("img")).not.toBeInTheDocument();
  });
});
