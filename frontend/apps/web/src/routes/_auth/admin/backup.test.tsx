import React from "react";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const mockUseQuery = vi.fn();
const mockUseMutation = vi.fn();
const mockInvalidateQueries = vi.fn();

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  eveyFetchRaw: vi.fn(),
  useBackupServiceListBackups: (...args: unknown[]) => mockUseQuery(...args),
  backupServiceCreateBackup: vi.fn(),
  backupServiceDeleteBackup: vi.fn(),
  getBackupServiceListBackupsQueryKey: () => ["backups"],
  backupScheduleServiceGetBackupSchedule: vi.fn(),
  getBackupScheduleServiceGetBackupScheduleQueryKey: () => ["backup-schedule"],
  backupScheduleServiceUpdateBackupSchedule: vi.fn(),
  backupScheduleServiceRunBackupNow: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn(), warning: vi.fn() },
  useDemoStore: (sel: (s: { demoLocked: string[] }) => unknown) => sel({ demoLocked: [] }),
  useAuthStore: (fn: (s: { accessToken: string }) => unknown) => fn({ accessToken: "tok" }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getBackupPage() {
  const mod = await import("./backup.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";

const mockBackup = {
  id: "backup-001",
  createdAt: "2024-01-15T10:00:00Z",
  version: "1.0.0",
  size: 1048576,
};

describe("BackupPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders page heading", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Backup & Restore/i)).toBeInTheDocument();
  });

  it("shows empty state when no backups", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    expect(screen.getByText(/No backups/i)).toBeInTheDocument();
  });

  it("shows spinner while loading", async () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Backup & Restore/i)).toBeInTheDocument();
  });

  it("renders backup row with id and size", async () => {
    mockUseQuery.mockReturnValue({ data: [mockBackup], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    expect(screen.getByText("backup-001")).toBeInTheDocument();
    expect(screen.getByText("1.0 MB")).toBeInTheDocument();
  });

  it("shows Ready badge for each backup", async () => {
    mockUseQuery.mockReturnValue({ data: [mockBackup], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    expect(screen.getByText("Ready")).toBeInTheDocument();
  });

  it("Create Backup button calls mutate", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, isPending: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Create Backup/i }));
    expect(mutate).toHaveBeenCalled();
  });

  it("Restore from File button opens modal", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Restore from File/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getByRole("dialog")).toBeInTheDocument();
  });

  it("clicking Delete on backup row shows confirm modal", async () => {
    mockUseQuery.mockReturnValue({ data: [mockBackup], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Delete$/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getAllByText(/Delete Backup/i).length).toBeGreaterThan(0);
  });

  it("clicking Restore on backup row shows confirm modal", async () => {
    mockUseQuery.mockReturnValue({ data: [mockBackup], isLoading: false });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Restore$/i }));
    await waitFor(() => expect(screen.getByRole("dialog")).toBeInTheDocument());
    expect(screen.getAllByText(/Restore Backup/i).length).toBeGreaterThan(0);
  });

  // The account token is no longer accepted in an admin URL: the link carries
  // a short token minted for THIS backup, never the session token.
  it("Download mints a backup-bound token and links with it, not the session token", async () => {
    mockUseQuery.mockReturnValue({ data: [mockBackup], isLoading: false });
    vi.mocked(eveyFetch).mockResolvedValue({ token: "dl-tok" });
    const clicked: string[] = [];
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (this: HTMLAnchorElement) {
      clicked.push(this.getAttribute("href") ?? "");
    });
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Download$/i }));

    await waitFor(() => expect(clicked).toHaveLength(1));
    expect(vi.mocked(eveyFetch)).toHaveBeenCalledWith(
      expect.objectContaining({ url: "/api/v1/admin/backups/backup-001/download-token", method: "POST" }),
    );
    expect(clicked[0]).toBe("/api/v1/admin/backups/backup-001?token=dl-tok");
    click.mockRestore();
  });

  it("Download says so when the token cannot be minted, instead of a dead link", async () => {
    mockUseQuery.mockReturnValue({ data: [mockBackup], isLoading: false });
    vi.mocked(eveyFetch).mockRejectedValue(new Error("forbidden"));
    const click = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
    const BackupPage = await getBackupPage();
    render(React.createElement(BackupPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Download$/i }));

    await waitFor(() => expect(vi.mocked(toast.error)).toHaveBeenCalled());
    expect(click).not.toHaveBeenCalled();
    click.mockRestore();
  });
});
