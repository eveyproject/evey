import { describe, it, expect, vi, beforeEach } from "vitest";
import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import React from "react";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { mutationErrorNet } from "@evey/client";
import { SmtpSettingsCard } from "./SmtpSettingsCard";

// Save writes six keys through the REAL config mutation. TanStack keeps the
// callbacks of the last mutate() call only: a per-call counter never reached
// zero (no "SMTP saved") while the hook toasted "config updated" six times.
const toast = vi.hoisted(() => ({ success: vi.fn(), error: vi.fn() }));
vi.mock("@evey/store", async (importOriginal) => ({ ...(await importOriginal<typeof import("@evey/store")>()), toast }));
const setEntry = vi.hoisted(() => vi.fn());
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  serverConfigServiceSetServerConfigEntry: (...a: unknown[]) => setEntry(...a),
}));
vi.mock("@/hooks/useDemoMode", () => ({ useDemoLock: () => ({ demo: false, lock: {} }) }));
vi.mock("react-i18next", () => ({ useTranslation: () => ({ t: (k: string, o?: Record<string, unknown>) => (o ? `${k}(${Object.values(o).join(",")})` : k) }) }));

function mount() {
  const qc = new QueryClient({ mutationCache: mutationErrorNet(), defaultOptions: { mutations: { retry: false } } });
  render(<QueryClientProvider client={qc}><SmtpSettingsCard entries={[]} /></QueryClientProvider>);
  fireEvent.click(screen.getByText("admin.serverConfig.saveSmtp"));
}

beforeEach(() => {
  vi.clearAllMocks();
  setEntry.mockResolvedValue({});
});

describe("SmtpSettingsCard save (real mutation)", () => {
  it("reports one success for the six writes, without a burst of config updated", async () => {
    mount();
    await waitFor(() => expect(toast.success).toHaveBeenCalledWith("admin.serverConfig.smtpSaved"));
    expect(setEntry).toHaveBeenCalledTimes(6);
    expect(toast.success).toHaveBeenCalledTimes(1);
    expect(toast.error).not.toHaveBeenCalled();
  });

  it("reports one failure naming the key that did not land, and no success", async () => {
    setEntry.mockImplementation(async (key: string) => {
      if (key === "smtp.password") throw new Error("boom");
      return {};
    });
    mount();
    await waitFor(() => expect(toast.error).toHaveBeenCalledWith("admin.serverConfig.failedSmtp(smtp.password)"));
    expect(toast.error).toHaveBeenCalledTimes(1);
    expect(toast.success).not.toHaveBeenCalled();
  });
});
