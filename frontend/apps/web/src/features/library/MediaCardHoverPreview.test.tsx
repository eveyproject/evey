import { describe, it, expect, vi } from "vitest";
import { render, screen } from "@testing-library/react";
import React from "react";
import { HoverPreview } from "./MediaCardHoverPreview";

vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
vi.mock("@/components/ui/GenreDot", () => ({ GenreDot: ({ label }: any) => React.createElement("i", { "data-testid": `dot-${label}` }) }));

// The hover card exists only when it has something to say (overview, rating
// or genres); it shows at most three genres, a rating only when positive
// with one decimal, the year and the kind when known.
describe("HoverPreview", () => {
  it("renders nothing for an item with no overview, rating or genres", () => {
    const { container } = render(<HoverPreview item={{ id: "1", title: "Bare", type: "MEDIA_ITEM_TYPE_MOVIE" } as any} />);
    expect(container.firstChild).toBeNull();
  });

  it("shows title, year, rating, kind, three genres at most and the overview", () => {
    render(
      <HoverPreview
        item={{
          id: "1", title: "Some Movie", type: "MEDIA_ITEM_TYPE_MOVIE", year: 1995, communityRating: 8.25,
          genres: ["Crime", "Drama", "Thriller", "Action"], overview: "A thief and a cop.",
        } as any}
      />,
    );
    expect(screen.getByText("Some Movie")).toBeInTheDocument();
    expect(screen.getByText("1995")).toBeInTheDocument();
    expect(screen.getByText("8.3")).toBeInTheDocument();
    expect(screen.getByText("Film")).toBeInTheDocument();
    expect(screen.getByText("A thief and a cop.")).toBeInTheDocument();
    expect(screen.getByTestId("dot-Crime")).toBeInTheDocument();
    expect(screen.getByTestId("dot-Thriller")).toBeInTheDocument();
    expect(screen.queryByTestId("dot-Action")).toBeNull();
  });

  it("hides a zero rating and an unknown kind", () => {
    render(<HoverPreview item={{ id: "1", title: "Track", type: "MEDIA_ITEM_TYPE_AUDIO_TRACK", communityRating: 0, overview: "x" } as any} />);
    expect(screen.queryByText("0.0")).toBeNull();
    expect(screen.queryByText("Film")).toBeNull();
    expect(screen.getByText("x")).toBeInTheDocument();
  });
});
