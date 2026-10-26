import { render, screen, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { PluginMetadataSettings } from "./PluginMetadataSettings";

const mockSave = vi.fn();
const mockRefresh = vi.fn();

type Refresh = { running?: boolean; done?: number; total?: number; covered?: number; failed?: number; finishedAt?: string };
let mockData: { fields?: string[]; availableFields?: string[]; refresh?: Refresh } | undefined;

vi.mock("@evey/api", () => ({
  useAdminPluginServiceGetPluginMetadataSettings: () => ({ data: mockData, isLoading: false }),
  useAdminPluginServiceUpdatePluginMetadataSettings: () => ({ mutate: mockSave, isPending: false }),
  useAdminPluginServiceRefreshPluginMetadata: () => ({ mutate: mockRefresh, isPending: false }),
  getAdminPluginServiceGetPluginMetadataSettingsQueryKey: (id: string) => ["metadata-settings", id],
  apiErrorMessage: (_e: unknown, _t: unknown, fallback: string) => fallback,
}));

vi.mock("@evey/store", () => ({ toast: { success: vi.fn(), error: vi.fn() } }));

vi.mock("@/hooks/useDemoMode", () => ({ useDemoLockFor: () => ({ locked: false, lock: {} }) }));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: vi.fn() }) };
});

describe("PluginMetadataSettings", () => {
  beforeEach(() => {
    mockSave.mockClear();
    mockRefresh.mockClear();
    mockData = { fields: ["overview", "tagline"], availableFields: ["title", "overview", "tagline"], refresh: {} };
  });

  it("shows the allowed fields and saves the new choice", () => {
    render(<PluginMetadataSettings pluginId="allocine" />);
    expect((screen.getByTestId("metadata-field-title") as HTMLInputElement).checked).toBe(false);
    expect((screen.getByTestId("metadata-field-overview") as HTMLInputElement).checked).toBe(true);

    fireEvent.click(screen.getByTestId("metadata-field-title"));
    fireEvent.click(screen.getByTestId("metadata-field-tagline"));
    fireEvent.click(screen.getByTestId("metadata-save"));
    expect(mockSave).toHaveBeenCalledWith({ pluginId: "allocine", data: { fields: ["overview", "title"] } });
  });

  it("starts the library fetch and shows its progress", () => {
    render(<PluginMetadataSettings pluginId="allocine" />);
    fireEvent.click(screen.getByTestId("metadata-refresh"));
    expect(mockRefresh).toHaveBeenCalledWith({ pluginId: "allocine", data: {} });
  });

  it("disables the fetch while one runs and reports the last one", () => {
    mockData = { ...mockData, refresh: { running: true, done: 3, total: 10 } };
    const { unmount } = render(<PluginMetadataSettings pluginId="allocine" />);
    expect(screen.getByTestId("metadata-refresh-running")).toBeTruthy();
    expect((screen.getByTestId("metadata-refresh") as HTMLButtonElement).disabled).toBe(true);
    unmount();

    mockData = { ...mockData, refresh: { done: 10, total: 10, covered: 8, failed: 1, finishedAt: "2026-10-26T12:00:00Z" } };
    render(<PluginMetadataSettings pluginId="allocine" />);
    expect(screen.getByTestId("metadata-refresh-finished")).toBeTruthy();
  });
});
