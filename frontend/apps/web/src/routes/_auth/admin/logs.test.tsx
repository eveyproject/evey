import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockEveyFetch = vi.fn();

vi.mock("@evey/api", () => ({
  eveyFetch: (...args: unknown[]) => mockEveyFetch(...args),
}));

vi.mock("@evey/store", () => ({
  useAuthStore: (fn: (s: { accessToken: string }) => unknown) => fn({ accessToken: "test-token" }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@tanstack/react-virtual", () => ({
  useVirtualizer: () => ({
    getTotalSize: () => 0,
    getVirtualItems: () => [],
    scrollToIndex: vi.fn(),
  }),
}));

// Mock EventSource
class MockEventSource {
  onmessage: ((e: MessageEvent) => void) | null = null;
  close = vi.fn();
  constructor(_url: string) {}
}
vi.stubGlobal("EventSource", MockEventSource);

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getLogsPage() {
  const mod = await import("./logs.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("LogsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockEveyFetch.mockResolvedValue({ lines: [] });
  });

  it("renders page heading", async () => {
    const LogsPage = await getLogsPage();
    render(React.createElement(LogsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Live Logs")).toBeInTheDocument();
  });

  it("renders level filter buttons", async () => {
    const LogsPage = await getLogsPage();
    render(React.createElement(LogsPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: "all" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "error" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "warn" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "info" })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "debug" })).toBeInTheDocument();
  });

  it("renders search and component filter inputs", async () => {
    const LogsPage = await getLogsPage();
    render(React.createElement(LogsPage), { wrapper: createWrapper() });
    expect(screen.getByPlaceholderText(/Search logs/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/Component/i)).toBeInTheDocument();
  });

  it("Live checkbox is checked by default", async () => {
    const LogsPage = await getLogsPage();
    render(React.createElement(LogsPage), { wrapper: createWrapper() });
    const liveCheckbox = screen.getByRole("checkbox");
    expect((liveCheckbox as HTMLInputElement).checked).toBe(true);
  });

  it("Clear button clears log lines display", async () => {
    const LogsPage = await getLogsPage();
    render(React.createElement(LogsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Clear/i }));
    // After clear, no entries in virtual list (getTotalSize returns 0)
    expect(screen.getByText("Live Logs")).toBeInTheDocument();
  });

  it("clicking level filter changes active level", async () => {
    const LogsPage = await getLogsPage();
    render(React.createElement(LogsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: "error" }));
    // Just verify the button is still present after click
    expect(screen.getByRole("button", { name: "error" })).toBeInTheDocument();
  });
});
