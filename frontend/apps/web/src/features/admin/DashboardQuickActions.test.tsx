import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor, act } from "@testing-library/react";
import React from "react";

vi.mock("@tanstack/react-router", () => ({ Link: ({ children, to }: any) => React.createElement("a", { href: to }, children) }));
const invalidate = vi.hoisted(() => vi.fn());
vi.mock("@tanstack/react-query", () => ({ useQueryClient: () => ({ invalidateQueries: invalidate }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string) => k }) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
const api = vi.hoisted(() => ({ libs: [] as unknown[], scan: vi.fn(), restart: vi.fn(), restartOpts: null as any }));
vi.mock("@evey/api", () => ({
  useLibraryServiceListLibraries: () => ({ data: { libraries: api.libs } }),
  useLibraryServiceScanLibrary: () => ({ mutateAsync: api.scan }),
  useAdminSystemServiceRestartServer: (opts: any) => {
    api.restartOpts = opts;
    return { mutate: api.restart, isPending: false };
  },
  apiErrorMessage: (e: Error, _t: unknown, f: string) => e.message || f,
}));
vi.mock("@/components/ui/Button", () => ({
  Button: ({ children, onClick, disabled, loading, variant }: any) => React.createElement("button", { onClick, disabled: disabled || loading, "data-variant": variant }, children),
}));
vi.mock("@/components/ui/Card", () => ({ Card: ({ children }: any) => React.createElement("div", null, children) }));

import { QuickActions } from "./DashboardQuickActions";

beforeEach(() => {
  vi.clearAllMocks();
  vi.useRealTimers();
  api.libs = [{ id: "l1" }, { id: "l2" }, { id: "" }];
  api.scan.mockResolvedValue({});
});

// "Scan all" walks every library one at a time and reports once at the end
// (a single failure makes the whole run a failure toast); restart is
// confirm-to-act - one click arms it for a few seconds, a second fires it -
// and an UNAVAILABLE answer means the deployment is not supervised.
describe("QuickActions", () => {
  it("scans every library sequentially and reports once", async () => {
    vi.spyOn(console, "warn").mockImplementation(() => {});
    render(<QuickActions />);
    fireEvent.click(screen.getByText("admin.dashboardStats.scanAllLibraries"));
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.dashboardStats.allScansTriggered"));
    expect(api.scan.mock.calls.map((c) => c[0])).toEqual([{ id: "l1" }, { id: "l2" }]);

    api.scan.mockRejectedValueOnce(new Error("busy"));
    fireEvent.click(screen.getByText("admin.dashboardStats.scanAllLibraries"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.dashboardStats.scanFailed"));
    expect(api.scan).toHaveBeenCalledTimes(4);
  });

  it("is dead without a library", () => {
    api.libs = [];
    render(<QuickActions />);
    expect((screen.getByText("admin.dashboardStats.scanAllLibraries") as HTMLButtonElement).disabled).toBe(true);
  });

  it("restart arms on the first click, disarms after 4 s, fires on the second", () => {
    vi.useFakeTimers();
    render(<QuickActions />);
    const btn = () => screen.getByText(/restartServer|restartConfirm/) as HTMLButtonElement;
    fireEvent.click(btn());
    expect(btn()).toHaveTextContent("admin.dashboardStats.restartConfirm");
    expect(btn().getAttribute("data-variant")).toBe("danger");
    expect(api.restart).not.toHaveBeenCalled();
    act(() => vi.advanceTimersByTime(4000));
    expect(btn()).toHaveTextContent("admin.dashboardStats.restartServer");

    fireEvent.click(btn());
    fireEvent.click(btn());
    expect(api.restart).toHaveBeenCalledWith({ data: {} });
    expect(btn()).toHaveTextContent("admin.dashboardStats.restartServer");

    api.restartOpts.mutation.onError({ code: 14 });
    expect(toast.error).toHaveBeenLastCalledWith("admin.dashboardStats.restartUnavailable");
    api.restartOpts.mutation.onError({ message: "boom" });
    expect(toast.error).toHaveBeenLastCalledWith("boom");
    api.restartOpts.mutation.onSuccess();
    expect(toast.success).toHaveBeenCalledWith("admin.dashboardStats.restartTriggered");
  });
});
