import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { themes, defaultTheme } from "@evey/theme";

// A builtin theme resolves straight from the catalogue; a plugin theme
// holds the default until its tokens arrive (no flash back to a builtin),
// then merges them; the plugin list is only fetched when a plugin theme is
// selected.
const store = vi.hoisted(() => ({ theme: "dark", pluginId: undefined as string | undefined }));
vi.mock("@evey/store", () => ({ useThemeStore: (sel: (s: typeof store) => unknown) => sel(store) }));
const eveyFetch = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetch(...a) }));

import { ThemeProvider, useTheme } from "./ThemeProvider";

function Probe() {
  const t = useTheme();
  return <span data-testid="accent">{t.colors.accent}</span>;
}
function mount() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(<QueryClientProvider client={qc}><ThemeProvider><Probe /></ThemeProvider></QueryClientProvider>);
}

beforeEach(() => {
  eveyFetch.mockReset().mockResolvedValue({ themes: [{ id: "ocean", name: "Ocean", tokens: { "--accent": "#0af" } }, { id: "bare", name: "Bare" }] });
  store.theme = "amoled";
  store.pluginId = undefined;
});

describe("ThemeProvider (mobile)", () => {
  it("resolves a builtin theme without asking the server, and falls back to the default for an unknown name", async () => {
    const first = mount();
    expect(screen.getByTestId("accent").textContent).toBe(themes.amoled.colors.accent);
    first.unmount();
    store.theme = "no-such-theme";
    mount();
    expect(screen.getByTestId("accent").textContent).toBe(defaultTheme.colors.accent);
    await new Promise((r) => setTimeout(r, 0));
    expect(eveyFetch).not.toHaveBeenCalled();
  });

  it("holds the default for a plugin theme until its tokens arrive, then merges them", async () => {
    store.theme = "plugin:ocean";
    store.pluginId = "ocean";
    mount();
    expect(screen.getByTestId("accent").textContent).toBe(defaultTheme.colors.accent);
    await waitFor(() => expect(screen.getByTestId("accent").textContent).toBe("#0af"));
    expect(eveyFetch).toHaveBeenCalledWith({ url: "/api/v1/plugins/themes", method: "GET" });
  });

  it("keeps the default for a plugin theme that ships no tokens", async () => {
    store.theme = "plugin:bare";
    store.pluginId = "bare";
    mount();
    await waitFor(() => expect(eveyFetch).toHaveBeenCalledTimes(1));
    await new Promise((r) => setTimeout(r, 0));
    expect(screen.getByTestId("accent").textContent).toBe(defaultTheme.colors.accent);
  });
});
