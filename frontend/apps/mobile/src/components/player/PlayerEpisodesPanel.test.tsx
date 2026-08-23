import { describe, it, expect, vi } from "vitest";
import { render, fireEvent } from "@testing-library/react";
import React from "react";

vi.mock("react-native", () => ({
  View: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("div", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Text: ({ children, style, ...rest }: Record<string, unknown>) =>
    React.createElement("span", { ...rest, "data-style": JSON.stringify(style) }, children as React.ReactNode),
  Pressable: ({ children, onPress, accessibilityLabel, accessibilityState, style, ...rest }: Record<string, unknown>) =>
    React.createElement(
      "button",
      {
        ...rest,
        onClick: onPress as () => void,
        "aria-label": accessibilityLabel as string,
        "aria-selected": (accessibilityState as { selected?: boolean } | undefined)?.selected,
        "data-style": JSON.stringify(style),
      },
      children as React.ReactNode,
    ),
  ScrollView: ({ children, style }: Record<string, unknown>) =>
    React.createElement("div", { "data-style": JSON.stringify(style) }, children as React.ReactNode),
  StyleSheet: { create: (s: Record<string, unknown>) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons", () => ({
  Ionicons: ({ name }: { name: string }) => React.createElement("span", { "data-icon": name }),
}));

vi.mock("../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      surfaceRaised: "#333",
      accent: "#818cf8",
      onAccent: "#000",
      text: "#fff",
      textMuted: "#888",
      border: "#222",
    },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

const TRANSLATIONS: Record<string, string> = {
  "player.controls.episodes": "Episodes",
};

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, opts?: Record<string, unknown> | string) => {
      if (key === "media.season") {
        const number = (opts as { number?: number } | undefined)?.number;
        return `Season ${number}`;
      }
      if (typeof opts === "string") return TRANSLATIONS[key] ?? opts;
      return TRANSLATIONS[key] ?? key;
    },
  }),
}));

import { PlayerEpisodesPanel } from "./PlayerEpisodesPanel";

const seasons = [
  { id: "s1", title: "Season 1", indexNumber: 1 },
  { id: "s2", title: "Season 2", indexNumber: 2 },
];

const season1Episodes = [
  { id: "e1", title: "Pilot", indexNumber: 1 },
  { id: "e2", title: "Second", indexNumber: 2 },
];

const season2Episodes = [
  { id: "e3", title: "Reunion", indexNumber: 1 },
];

describe("PlayerEpisodesPanel", () => {
  it("hides the season selector when there is only one season", () => {
    const { queryByLabelText } = render(
      <PlayerEpisodesPanel
        seasons={[seasons[0]]}
        currentSeasonId="s1"
        episodes={season1Episodes}
        currentEpisodeId="e1"
        onSeasonChange={vi.fn()}
        onEpisodeChange={vi.fn()}
      />,
    );
    expect(queryByLabelText("Season 1")).toBeNull();
  });

  it("lists episodes and highlights the current one", () => {
    const { getByText, getByLabelText } = render(
      <PlayerEpisodesPanel
        seasons={seasons}
        currentSeasonId="s1"
        episodes={season1Episodes}
        currentEpisodeId="e1"
        onSeasonChange={vi.fn()}
        onEpisodeChange={vi.fn()}
      />,
    );
    expect(getByText("Pilot")).toBeTruthy();
    expect(getByText("Second")).toBeTruthy();
    expect(getByLabelText("Pilot").getAttribute("aria-selected")).toBe("true");
    expect(getByLabelText("Second").getAttribute("aria-selected")).toBe("false");
  });

  it("switching season reports the new list via onSeasonChange", () => {
    const onSeasonChange = vi.fn();
    const { getByLabelText } = render(
      <PlayerEpisodesPanel
        seasons={seasons}
        currentSeasonId="s1"
        episodes={season1Episodes}
        currentEpisodeId="e1"
        onSeasonChange={onSeasonChange}
        onEpisodeChange={vi.fn()}
      />,
    );
    fireEvent.click(getByLabelText("Season 2"));
    expect(onSeasonChange).toHaveBeenCalledWith("s2");
  });

  it("renders another season's episodes once the caller switches episodes", () => {
    const { getByText, queryByText } = render(
      <PlayerEpisodesPanel
        seasons={seasons}
        currentSeasonId="s2"
        episodes={season2Episodes}
        currentEpisodeId="e1"
        onSeasonChange={vi.fn()}
        onEpisodeChange={vi.fn()}
      />,
    );
    expect(getByText("Reunion")).toBeTruthy();
    expect(queryByText("Pilot")).toBeNull();
  });

  // A season has no name: the chip label comes from the number. Rows enriched
  // before 8a09537ea still carry a French "Saison 2" in the database.
  it("derives a season chip from its number, not from the stored title", () => {
    const stored = [
      { id: "s1", title: "Saison 1", indexNumber: 1 },
      { id: "s2", title: "Saison 2", indexNumber: 2 },
    ];
    const { getByText, queryByText } = render(
      <PlayerEpisodesPanel
        seasons={stored}
        currentSeasonId="s1"
        episodes={season1Episodes}
        currentEpisodeId="e1"
        onSeasonChange={vi.fn()}
        onEpisodeChange={vi.fn()}
      />,
    );
    expect(getByText("Season 2")).toBeTruthy();
    expect(queryByText("Saison 2")).toBeNull();
  });

  // No index_number (a search hit has none): the stored title stays rather
  // than a chip reading "Season undefined".
  it("falls back to the stored title when a season has no number", () => {
    const stored = [
      { id: "s1", title: "First batch" },
      { id: "s2", title: "Second batch" },
    ];
    const { getByText } = render(
      <PlayerEpisodesPanel
        seasons={stored}
        currentSeasonId="s1"
        episodes={season1Episodes}
        currentEpisodeId="e1"
        onSeasonChange={vi.fn()}
        onEpisodeChange={vi.fn()}
      />,
    );
    expect(getByText("Second batch")).toBeTruthy();
  });

  it("tapping an episode from another season navigates to it", () => {
    const onEpisodeChange = vi.fn();
    const { getByText } = render(
      <PlayerEpisodesPanel
        seasons={seasons}
        currentSeasonId="s2"
        episodes={season2Episodes}
        currentEpisodeId="e1"
        onSeasonChange={vi.fn()}
        onEpisodeChange={onEpisodeChange}
      />,
    );
    fireEvent.click(getByText("Reunion"));
    expect(onEpisodeChange).toHaveBeenCalledWith("e3");
  });
});
