import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

// Guards the admin log explorer on the phone: rows list what the server
// returned, an opened row names its user and the title behind its media id
// (resolved only then, so a long list costs no lookup), and a failed search
// says so instead of reading as an empty log.

vi.mock("react-native", () => ({
  View: ({ children, ...p }: any) => React.createElement("div", p, children),
  Text: ({ children, ...p }: any) => React.createElement("span", p, children),
  TextInput: (p: any) => React.createElement("input", p),
  Pressable: ({ children, onPress, ...p }: any) => React.createElement("button", { onClick: onPress, ...p }, children),
  ActivityIndicator: () => React.createElement("div", { "data-testid": "loading" }),
  ScrollView: ({ children }: any) => React.createElement("div", null, children),
  FlatList: ({ data, renderItem, ListHeaderComponent, ListEmptyComponent }: any) =>
    React.createElement(
      "div",
      null,
      ListHeaderComponent,
      data.length === 0 ? ListEmptyComponent : data.map((item: any, index: number) =>
        React.createElement(React.Fragment, { key: item.uid ?? index }, renderItem({ item, index }))),
    ),
  Share: { share: vi.fn() },
  StyleSheet: { create: (s: any) => s, hairlineWidth: 1 },
}));

vi.mock("@expo/vector-icons/Ionicons", () => ({
  default: ({ name }: any) => React.createElement("span", { "data-testid": `icon-${name}` }),
}));

vi.mock("expo-router", () => ({ Stack: { Screen: () => null } }));

vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
  }),
}));

vi.mock("../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: {
      bg: "#000", surface: "#111", surfaceRaised: "#222", text: "#fff", textMuted: "#888",
      accent: "#f00", border: "#333", error: "#f00", warning: "#fa0", success: "#0f0", info: "#00f",
    },
    radii: { sm: 4, md: 8 },
  }),
}));

const api = vi.hoisted(() => ({
  search: vi.fn(),
  getMediaItem: vi.fn(),
}));

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => api.search(...args),
  libraryServiceGetMediaItem: (...args: unknown[]) => api.getMediaItem(...args),
  useUserServiceListUsers: () => ({ data: { users: [{ id: "usr-1", username: "someone" }] } }),
  useUserServiceListAllProfiles: () => ({ data: { profiles: [] } }),
}));

import { useDemoStore } from "@evey/store";
import LogsScreen from "../../app/(auth)/admin/logs";

const entry = {
  uid: "l1",
  time: "2026-09-26T10:00:00Z",
  level: "WARN",
  msg: "player stalled",
  where: "player",
  source: "client",
  mediaId: "mi-1",
  userId: "usr-1",
  raw: "{}",
};

function renderScreen() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(LogsScreen)));
}

describe("Admin logs screen (mobile)", () => {
  beforeEach(() => {
    useDemoStore.setState({ demoMode: false, demoLocked: [] });
    api.search.mockReset();
    api.getMediaItem.mockReset();
    api.getMediaItem.mockResolvedValue({ id: "mi-1", type: "MEDIA_ITEM_TYPE_MOVIE", title: "Some Movie", year: 2016 });
  });

  it("names the user and the title behind the media id once a row is opened", async () => {
    api.search.mockResolvedValue({ entries: [entry], total: 1, truncated: false });
    renderScreen();

    fireEvent.click(await screen.findByText("player stalled"));

    expect(await screen.findByText("user: someone (usr-1)")).toBeTruthy();
    expect(await screen.findByText("media: Some Movie (2016) (mi-1)")).toBeTruthy();
  });

  it("does not look up a media title while its row stays closed", async () => {
    api.search.mockResolvedValue({ entries: [entry], total: 1, truncated: false });
    renderScreen();

    await screen.findByText("player stalled");
    await new Promise((r) => setTimeout(r, 0));
    expect(api.getMediaItem).not.toHaveBeenCalled();
  });

  it("says the search failed instead of showing an empty log", async () => {
    api.search.mockRejectedValue(new Error("boom"));
    renderScreen();

    await waitFor(() => expect(screen.getByText("admin.logs.error")).toBeTruthy());
    expect(screen.queryByText("admin.logs.empty")).toBeNull();
  });

  it("shows the locked state and never searches when the server lists the log routes as locked", async () => {
    useDemoStore.setState({ demoLocked: ["GET /api/v1/admin/logs/search"] });
    renderScreen();

    expect(await screen.findByText("errors.demo.locked")).toBeTruthy();
    await new Promise((r) => setTimeout(r, 0));
    expect(api.search).not.toHaveBeenCalled();
  });
});
