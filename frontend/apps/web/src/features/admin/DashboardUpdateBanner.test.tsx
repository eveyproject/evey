import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { DashboardUpdateBanner } from "./DashboardUpdateBanner";

const mockFetch = vi.fn();
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: (...args: unknown[]) => mockFetch(...args),
}));

function renderBanner() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false, gcTime: 0 } } });
  return render(
    <QueryClientProvider client={qc}>
      <DashboardUpdateBanner />
    </QueryClientProvider>,
  );
}

describe("DashboardUpdateBanner", () => {
  beforeEach(() => mockFetch.mockReset());

  it("shows the newer build, the command to run and what changed", async () => {
    mockFetch.mockResolvedValue({
      state: "update_available",
      currentVersion: "beta-0.0.1-full",
      currentCommit: "cbcd835ec767",
      latestVersion: "beta-0.0.1-full",
      latestCommit: "e77dc0a31fff",
      source: "https://github.com/eveyproject/evey",
    });
    renderBanner();
    const banner = await screen.findByTestId("update-banner");
    expect(banner).toHaveTextContent("e77dc0a");
    expect(banner).toHaveTextContent("docker compose pull && docker compose up -d");
    expect(screen.getByRole("link")).toHaveAttribute("href", "https://github.com/eveyproject/evey/compare/cbcd835ec767...e77dc0a31fff");
  });

  it("stays out of the way when up to date or unknown", async () => {
    mockFetch.mockResolvedValue({ state: "up_to_date", currentVersion: "1.0.0", currentCommit: "x" });
    renderBanner();
    await new Promise((r) => setTimeout(r, 20));
    expect(screen.queryByTestId("update-banner")).toBeNull();
  });
});
