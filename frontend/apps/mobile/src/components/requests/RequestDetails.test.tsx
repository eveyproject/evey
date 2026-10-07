import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent } from "@testing-library/react";
import React from "react";

const openURL = vi.hoisted(() => vi.fn(() => Promise.resolve(true)));

vi.mock("react-native", () => ({
  View: ({ children, accessibilityLabel, testID, ...p }: any) =>
    React.createElement("div", { "aria-label": accessibilityLabel, "data-testid": testID, ...p }, children),
  Text: ({ children, numberOfLines: _n, testID, ...p }: any) => React.createElement("span", { "data-testid": testID, ...p }, children),
  Pressable: ({ children, onPress, accessibilityLabel, testID, hitSlop: _h, ...p }: any) =>
    React.createElement("button", { onClick: onPress, "aria-label": accessibilityLabel, "data-testid": testID, ...p }, children),
  Linking: { openURL },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("i", { "data-icon": name }),
}));

// The viewer reads French: the TMDB link must land on the French page.
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown>) => {
      if (key === "requests.openOn") return `Ouvrir sur ${opts?.site}`;
      if (key === "requests.year") return `(${opts?.year})`;
      return key;
    },
    i18n: { language: "fr" },
  }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { text: "#fff", textMuted: "#aaa", warning: "#fc0" },
    radii: { sm: 4 },
  }),
}));

import { RequestFacts, RequestOverview, RequestTitleLink } from "./RequestDetails";

describe("RequestTitleLink", () => {
  it("opens the provider's page in the viewer's language when pressed", () => {
    render(<RequestTitleLink title={{ provider: "tmdb", providerId: "1399", mediaType: "tv" }} text="Thrones" year={2011} />);
    const link = screen.getByTestId("request-external-link");
    expect(link.getAttribute("aria-label")).toBe("Ouvrir sur TMDB");
    expect(link.textContent).toContain("Thrones (2011)");
    fireEvent.click(link);
    expect(openURL).toHaveBeenCalledWith("https://www.themoviedb.org/tv/1399?language=fr-FR");
  });

  it("is plain text for a free-text request, which has no page to open", () => {
    render(<RequestTitleLink title={{ provider: "", providerId: "" }} text="Nowhere" />);
    expect(screen.queryByTestId("request-external-link")).toBeNull();
    expect(screen.getByText("Nowhere")).toBeTruthy();
  });
});

describe("RequestFacts and RequestOverview", () => {
  it("shows the date and the rating, and nothing when both are unknown", () => {
    const { rerender } = render(<RequestFacts releaseDate="2010-07-16" rating={8.37} />);
    expect(screen.getByTestId("request-facts").textContent).toContain("8.4");
    expect(screen.getByTestId("request-facts").textContent).toContain("2010");
    rerender(<RequestFacts releaseDate="" rating={0} />);
    expect(screen.queryByTestId("request-facts")).toBeNull();
  });

  it("shows the synopsis and hides a blank one", () => {
    const { rerender } = render(<RequestOverview overview="A heist inside a dream." />);
    expect(screen.getByTestId("request-overview").textContent).toBe("A heist inside a dream.");
    rerender(<RequestOverview overview="  " />);
    expect(screen.queryByTestId("request-overview")).toBeNull();
  });
});
