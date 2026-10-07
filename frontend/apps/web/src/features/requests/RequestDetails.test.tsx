import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi } from "vitest";

// The viewer reads French: the TMDB link must land on the French page.
vi.mock("react-i18next", async (importActual) => ({
  ...(await importActual<typeof import("react-i18next")>()),
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === "requests.openOn") return `Ouvrir sur ${opts?.site}`;
      if (key === "requests.ratingLabel") return `Note ${opts?.rating} sur 10`;
      if (key === "requests.releaseDateLabel") return `Date de sortie : ${opts?.date}`;
      return key;
    },
    i18n: { language: "fr" },
  }),
}));

import { ExternalTitleLink, RequestFacts, RequestOverview } from "./RequestDetails";

describe("ExternalTitleLink", () => {
  it("opens the provider's page in a new tab, in the viewer's language", () => {
    render(
      <ExternalTitleLink title={{ provider: "tmdb", providerId: "550", mediaType: "movie" }}>Fight Club</ExternalTitleLink>,
    );
    const link = screen.getByTestId("request-external-link");
    expect(link).toHaveAttribute("href", "https://www.themoviedb.org/movie/550?language=fr-FR");
    expect(link).toHaveAttribute("target", "_blank");
    expect(link).toHaveAttribute("rel", "noopener noreferrer");
    expect(link).toHaveAttribute("aria-label", "Ouvrir sur TMDB");
    expect(link).toHaveTextContent("Fight Club");
  });

  it("is plain text for a free-text request, which has no page to open", () => {
    render(<ExternalTitleLink title={{ provider: "", providerId: "", mediaType: "movie" }}>Nowhere</ExternalTitleLink>);
    expect(screen.queryByTestId("request-external-link")).toBeNull();
    expect(screen.getByText("Nowhere")).toBeInTheDocument();
  });
});

describe("RequestFacts", () => {
  it("shows the release date in the viewer's locale and the rating with one decimal", () => {
    render(<RequestFacts releaseDate="1999-10-15" rating={8.43} />);
    const facts = screen.getByTestId("request-facts");
    expect(facts).toHaveTextContent("15 oct. 1999");
    expect(facts).toHaveTextContent("8.4");
  });

  it("renders nothing when the provider gave neither", () => {
    render(<RequestFacts releaseDate="" rating={0} />);
    expect(screen.queryByTestId("request-facts")).toBeNull();
  });
});

describe("RequestOverview", () => {
  it("shows the synopsis and nothing for a blank one", () => {
    const { rerender } = render(<RequestOverview overview="An insomniac office worker..." />);
    expect(screen.getByTestId("request-overview")).toHaveTextContent("An insomniac office worker...");
    rerender(<RequestOverview overview="   " />);
    expect(screen.queryByTestId("request-overview")).toBeNull();
  });
});
