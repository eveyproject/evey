import React from "react";
import { render, screen } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";

const mockEveyFetch = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PluginSecurityDashboard } from "./PluginSecurityDashboard";

function renderDashboard() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}>
      <PluginSecurityDashboard />
    </QueryClientProvider>,
  );
}

const landlockSandbox = {
  configured: "auto",
  effective: "landlock",
  landlockKernel: true,
  launcherPresent: true,
  bubblewrapUsable: false,
};

const basicSandbox = {
  configured: "auto",
  effective: "basic",
  landlockKernel: false,
  launcherPresent: true,
  bubblewrapUsable: false,
};

describe("PluginSecurityDashboard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("shows the host sandbox card even with zero plugins installed", async () => {
    mockEveyFetch.mockResolvedValue({ plugins: [], sandbox: landlockSandbox });
    renderDashboard();
    expect(await screen.findByTestId("sandbox-status-card")).toBeInTheDocument();
    expect(screen.getByText("landlock")).toBeInTheDocument();
    // auto → landlock: the configured mode is shown alongside the resolved one.
    expect(screen.getByText(/resolved from auto/i)).toBeInTheDocument();
    // Healthy host → no degradation warning.
    expect(screen.queryByTestId("sandbox-degraded-warning")).not.toBeInTheDocument();
    expect(screen.getByText(/No security data available/i)).toBeInTheDocument();
  });

  it("warns with probe results when the sandbox degraded to basic", async () => {
    mockEveyFetch.mockResolvedValue({ plugins: [], sandbox: basicSandbox });
    renderDashboard();
    const warning = await screen.findByTestId("sandbox-degraded-warning");
    expect(warning).toHaveTextContent(/UID-drop \+ seccomp only/i);
    expect(warning).toHaveTextContent(/Landlock kernel/i);
    expect(warning).toHaveTextContent(/kernel ≥ 5\.13/i);
  });

  it("renders plugin rows with their effective sandbox badge", async () => {
    mockEveyFetch.mockResolvedValue({
      sandbox: landlockSandbox,
      plugins: [
        {
          id: "p1",
          name: "Weather",
          version: "1.0.0",
          sandboxMode: "landlock",
          permissions: [],
          findings: [],
          sourceSHA256: "cafe1234",
          installedAt: "2026-01-01T00:00:00Z",
          sizeBytes: 1024,
        },
      ],
    });
    renderDashboard();
    expect(await screen.findByText("Weather")).toBeInTheDocument();
    // Host card + plugin row both show "landlock".
    expect(screen.getAllByText("landlock")).toHaveLength(2);
    expect(screen.queryByText(/No security data available/i)).not.toBeInTheDocument();
  });

  // A 500 or a dropped connection used to read as "no data", hiding the
  // degraded-sandbox warning behind an empty-looking dashboard.
  it("shows an error with Retry when the read fails, not the empty state", async () => {
    mockEveyFetch.mockRejectedValue(new Error("500"));
    renderDashboard();
    expect(await screen.findByTestId("fetch-error-state")).toBeInTheDocument();
    expect(screen.queryByText(/No security data available/i)).not.toBeInTheDocument();
    mockEveyFetch.mockResolvedValue({ plugins: [], sandbox: basicSandbox });
    screen.getByRole("button", { name: /Retry/i }).click();
    expect(await screen.findByTestId("sandbox-degraded-warning")).toBeInTheDocument();
  });
});
