import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const eveyFetchMock = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", () => ({ eveyFetch: (...a: unknown[]) => eveyFetchMock(...a) }));
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", () => ({ toast }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, o?: { name?: string }) => (o ? `${k}(${o.name})` : k) }) }));
vi.mock("./users-constants", () => ({ BITRATE_OPTIONS: [{ label: "Unlimited", value: 0 }, { label: "4 Mbps", value: 4000 }, { label: "8 Mbps", value: 8000 }] }));
vi.mock("@/components/ui/Modal", () => ({ Modal: ({ open, children, title }: any) => (open ? React.createElement("div", { role: "dialog", "aria-label": title }, children) : null) }));
vi.mock("@/components/ui/Spinner", () => ({ Spinner: () => React.createElement("div", { "data-testid": "spinner" }) }));
vi.mock("@/components/ui/Button", () => ({ Button: ({ children, onClick, loading }: any) => React.createElement("button", { onClick, disabled: loading }, children) }));
vi.mock("@/components/ui/Select", () => ({
  Select: ({ label, value, onChange, options }: any) =>
    React.createElement("select", { "aria-label": label, value, onChange: (e: React.ChangeEvent<HTMLSelectElement>) => onChange(e.target.value) }, options.map((o: any) => React.createElement("option", { key: o.value, value: o.value }, o.label))),
}));

import { BitrateModal } from "./BitrateModal";

function mount(open = true) {
  const onClose = vi.fn();
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  render(React.createElement(QueryClientProvider, { client: qc }, React.createElement(BitrateModal, { user: { id: "u1", username: "alice" } as any, open, onClose })));
  return onClose;
}

beforeEach(() => {
  vi.clearAllMocks();
  eveyFetchMock.mockImplementation((opts: { method: string }) => Promise.resolve(opts.method === "GET" ? { max_bitrate_kbps: 4000 } : undefined));
});

// The modal reads the user's current cap only once opened, seeds the select
// from it, PATCHes the picked value as a number and closes; a refused save
// stays open and says so.
describe("BitrateModal", () => {
  it("fetches only when open, seeds from the cap, saves as a number and closes", async () => {
    mount(false);
    expect(eveyFetchMock).not.toHaveBeenCalled();
    const onClose = mount(true);
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/users/u1/bitrate-limit", method: "GET" }));
    const select = await screen.findByLabelText("admin.users.maxBitrate") as HTMLSelectElement;
    await waitFor(() => expect(select.value).toBe("4000"));
    fireEvent.change(select, { target: { value: "8000" } });
    fireEvent.click(screen.getByText("common.save"));
    await waitFor(() => expect(eveyFetchMock).toHaveBeenCalledWith({ url: "/api/v1/admin/users/u1/bitrate-limit", method: "PATCH", data: { max_bitrate_kbps: 8000 } }));
    await waitFor(() => expect(onClose).toHaveBeenCalled());
    expect(toast.success).toHaveBeenCalledWith("admin.users.bitrateUpdated");
    expect(screen.getByRole("dialog").getAttribute("aria-label")).toBe("admin.users.maxBitrateTitle(alice)");
  });

  it("a refused save stays open and says so", async () => {
    eveyFetchMock.mockImplementation((opts: { method: string }) => (opts.method === "GET" ? Promise.resolve({ max_bitrate_kbps: 0 }) : Promise.reject(new Error("403"))));
    const onClose = mount(true);
    await screen.findByLabelText("admin.users.maxBitrate");
    fireEvent.click(screen.getByText("common.save"));
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.users.failedBitrate"));
    expect(onClose).not.toHaveBeenCalled();
  });
});
