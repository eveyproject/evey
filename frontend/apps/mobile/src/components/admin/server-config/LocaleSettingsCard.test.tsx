import { describe, it, expect, vi } from "vitest";
import { render, screen, fireEvent, within } from "@testing-library/react";
import React from "react";

vi.mock("react-native", async () => {
  const { rnDom } = await import("../../../test/rnDom");
  return {
    ...rnDom,
    FlatList: ({ data, renderItem, keyExtractor }: any) =>
      React.createElement(
        "div",
        { "data-testid": "region-list" },
        (data ?? []).map((item: any) => React.createElement(React.Fragment, { key: keyExtractor(item) }, renderItem({ item }))),
      ),
  };
});

// The defaults the server reports for empty settings.
vi.mock("@tanstack/react-query", () => ({
  useQuery: () => ({ data: { "server.language": "en", "metadata.language": "en", "server.timezone": "Europe/Paris" } }),
}));
vi.mock("react-i18next", () => ({
  useTranslation: () => ({
    t: (key: string, fallback?: unknown) => (typeof fallback === "string" ? fallback : key),
    i18n: { language: "en" },
  }),
}));

vi.mock("../../../providers/ThemeProvider", () => ({
  useTheme: () => ({
    colors: { border: "#333", surface: "#111", accent: "#f00", onAccent: "#fff", text: "#fff", textMuted: "#888", bg: "#000" },
    radii: { sm: 4, md: 8, lg: 12 },
  }),
}));

// Real module init side effects (i18next backend, secure-storage-backed
// zustand persistence) are irrelevant here - only the static language list
// is used by this card.
vi.mock("../../../i18n", () => ({
  supportedLanguages: [
    { code: "en", label: "English" },
    { code: "fr", label: "Français" },
  ],
}));

import { LocaleSettingsCard } from "./LocaleSettingsCard";

describe("LocaleSettingsCard", () => {
  it("saves server.language when a language chip is pressed", () => {
    // "Français" appears both in the server-language and metadata-language
    // rows; the server-language ChipSelect renders first.
    const saveEntry = vi.fn();
    render(<LocaleSettingsCard getEntryValue={() => ""} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getAllByText("Français")[0]);
    expect(saveEntry).toHaveBeenCalledWith({ key: "server.language", value: "fr" });
  });

  it("saves metadata.language independently of server.language", () => {
    const saveEntry = vi.fn();
    render(<LocaleSettingsCard getEntryValue={() => ""} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getAllByText("Français")[1]);
    expect(saveEntry).toHaveBeenCalledWith({ key: "metadata.language", value: "fr" });
  });

  it("opens the region picker and saves server.region on selection", () => {
    // Region labels come from Intl.DisplayNames, which falls back to the raw
    // ISO code ("FR") when the test runtime lacks full ICU data - matching
    // that fallback here instead of the localized "France" keeps the test
    // independent of the runtime's ICU build.
    const saveEntry = vi.fn();
    render(<LocaleSettingsCard getEntryValue={() => ""} saveEntry={saveEntry} saving={false} />);

    expect(screen.queryByTestId("region-list")).toBeNull();
    fireEvent.click(screen.getByLabelText("Server region"));
    expect(screen.getByTestId("region-list")).toBeTruthy();

    fireEvent.click(screen.getAllByText("FR")[0]);
    expect(saveEntry).toHaveBeenCalledWith({ key: "server.region", value: "FR" });
  });

  it("does NOT apply the language to the admin's own UI locale", () => {
    // Regression guard: the web card calls useLocaleStore().setLocale after
    // saving server.language; mobile precedence (profile > user > device >
    // server) means this card must never touch it. useLocaleStore isn't even
    // imported here - if it were, it would need @evey/store mocked, and this
    // module import would already have failed without that mock.
    const saveEntry = vi.fn();
    render(<LocaleSettingsCard getEntryValue={() => ""} saveEntry={saveEntry} saving={false} />);
    fireEvent.click(screen.getAllByText("Français")[0]);
    expect(saveEntry).toHaveBeenCalledTimes(1);
  });

  it("gives each of the four settings exactly one info button", () => {
    render(<LocaleSettingsCard getEntryValue={() => ""} saveEntry={vi.fn()} saving={false} />);
    const rows = ["server-language", "server-region", "metadata-language", "server-timezone"];
    for (const id of rows) {
      const row = screen.getByTestId(`setting-row-${id}`);
      expect(within(row).getAllByTestId("setting-info-button")).toHaveLength(1);
    }
    expect(screen.getAllByTestId("setting-info-button")).toHaveLength(rows.length);
  });
});
