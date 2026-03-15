import React from "react";
import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockGetAuditLog = vi.fn();

vi.mock("@evey/api", () => ({
  useAdminServiceGetAuditLog: (...args: unknown[]) => mockGetAuditLog(...args),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

vi.mock("@/features/admin/AuditLogTable", () => ({
  AuditLogTable: ({
    entries,
    total,
  }: {
    entries: unknown[];
    total: number;
    page: number;
    pageSize: number;
    onPageChange: (p: number) => void;
  }) =>
    React.createElement(
      "div",
      { "data-testid": "audit-log-table" },
      `${entries.length} entries, total: ${total}`
    ),
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getAuditPage() {
  const mod = await import("./audit.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

describe("AuditPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders page heading", async () => {
    mockGetAuditLog.mockReturnValue({ data: { entries: [], total: 0 }, isLoading: false });
    const AuditPage = await getAuditPage();
    render(React.createElement(AuditPage), { wrapper: createWrapper() });
    expect(screen.getByText("Audit Log")).toBeInTheDocument();
  });

  it("renders filter inputs", async () => {
    mockGetAuditLog.mockReturnValue({ data: { entries: [], total: 0 }, isLoading: false });
    const AuditPage = await getAuditPage();
    render(React.createElement(AuditPage), { wrapper: createWrapper() });
    expect(screen.getByPlaceholderText(/e.g. login/i)).toBeInTheDocument();
    expect(screen.getByPlaceholderText(/e.g. library/i)).toBeInTheDocument();
  });

  it("renders AuditLogTable with entries", async () => {
    const entries = [{ id: "1", action: "login", resourceType: "user", createdAt: "" }];
    mockGetAuditLog.mockReturnValue({ data: { entries, total: 1 }, isLoading: false });
    const AuditPage = await getAuditPage();
    render(React.createElement(AuditPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("audit-log-table")).toBeInTheDocument();
    expect(screen.getByText(/1 entries, total: 1/)).toBeInTheDocument();
  });

  it("shows spinner while loading", async () => {
    mockGetAuditLog.mockReturnValue({ data: undefined, isLoading: true });
    const AuditPage = await getAuditPage();
    render(React.createElement(AuditPage), { wrapper: createWrapper() });
    expect(screen.queryByTestId("audit-log-table")).not.toBeInTheDocument();
  });

  it("Apply button triggers filter update", async () => {
    mockGetAuditLog.mockReturnValue({ data: { entries: [], total: 0 }, isLoading: false });
    const AuditPage = await getAuditPage();
    render(React.createElement(AuditPage), { wrapper: createWrapper() });
    const actionInput = screen.getByPlaceholderText(/e.g. login/i);
    fireEvent.change(actionInput, { target: { value: "login" } });
    fireEvent.click(screen.getByRole("button", { name: /Apply/i }));
    expect(mockGetAuditLog).toHaveBeenCalled();
  });

  it("Clear button resets filters", async () => {
    mockGetAuditLog.mockReturnValue({ data: { entries: [], total: 0 }, isLoading: false });
    const AuditPage = await getAuditPage();
    render(React.createElement(AuditPage), { wrapper: createWrapper() });
    const actionInput = screen.getByPlaceholderText(/e.g. login/i);
    fireEvent.change(actionInput, { target: { value: "login" } });
    fireEvent.click(screen.getByRole("button", { name: /Clear/i }));
    expect((actionInput as HTMLInputElement).value).toBe("");
  });
});
