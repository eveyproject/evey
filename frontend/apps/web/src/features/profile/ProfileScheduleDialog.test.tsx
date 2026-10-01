import React from "react";
import { render, screen, fireEvent, act, waitFor, within } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";

const api = vi.hoisted(() => ({ get: vi.fn(), put: vi.fn() }));
vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  getProfileSchedule: (...a: unknown[]) => api.get(...a),
  updateProfileSchedule: (...a: unknown[]) => api.put(...a),
}));
vi.mock("@/hooks/useDemoMode", () => ({ useDemoLock: () => ({ demo: false, lock: {} }) }));

import { ProfileScheduleDialog } from "./ProfileScheduleDialog";

const proposal = {
  enabled: true,
  mode: "week" as const,
  rules: [
    { days: [1, 2, 3, 4, 5], ranges: [{ start: 1080, end: 1320 }] },
    { days: [6, 0], ranges: [{ start: 540, end: 720 }, { start: 840, end: 1260 }] },
  ],
};
const state = (schedule: typeof proposal | null) => ({
  isKids: true,
  schedule,
  proposal,
  access: { open: true, overridden: false },
  timezone: "Europe/Paris",
});

function renderDialog() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  const onClose = vi.fn();
  render(
    <QueryClientProvider client={qc}>
      <ProfileScheduleDialog open profileId="kid" profileName="Léo" onClose={onClose} />
    </QueryClientProvider>,
  );
  return { onClose };
}

beforeEach(() => {
  api.get.mockReset();
  api.put.mockReset();
});

describe("ProfileScheduleDialog", () => {
  // Nothing saved yet: the suggested hours are shown, off, and turning them
  // on saves exactly the proposal.
  it("starts from the proposal and saves it once enabled", async () => {
    api.get.mockResolvedValue(state(null));
    api.put.mockResolvedValue(state(proposal));
    const { onClose } = renderDialog();
    await screen.findByText("Monday to Friday");
    expect(screen.getByText("Saturday and Sunday")).toBeInTheDocument();
    expect(screen.getByText(/Europe\/Paris/)).toBeInTheDocument();

    fireEvent.click(screen.getByRole("switch"));
    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
    });
    expect(api.put).toHaveBeenCalledWith("kid", proposal);
    await waitFor(() => expect(onClose).toHaveBeenCalled());
  });

  // Day by day lists the seven days; a range can be added to one of them,
  // and the week view then refuses rather than dropping it.
  it("edits day by day and refuses a lossy switch back to the week view", async () => {
    api.get.mockResolvedValue(state(proposal));
    api.put.mockResolvedValue(state(proposal));
    renderDialog();
    await screen.findByText("Monday to Friday");

    fireEvent.click(screen.getByRole("radio", { name: "Day by day" }));
    expect(screen.getByText("Wednesday")).toBeInTheDocument();
    const wednesday = screen.getByText("Wednesday").closest("div")!.parentElement!;
    fireEvent.click(within(wednesday).getByRole("button", { name: "Add a time range" }));

    fireEvent.click(screen.getByRole("radio", { name: "Weekdays / weekend" }));
    expect(screen.getByRole("alert")).toHaveTextContent("Some days differ");
    expect(screen.getByRole("radio", { name: "Day by day" })).toHaveAttribute("aria-checked", "true");

    await act(async () => {
      fireEvent.click(screen.getByRole("button", { name: "Save" }));
    });
    const sent = api.put.mock.calls[0][1];
    expect(sent.mode).toBe("daily");
    expect(sent.rules.find((r: { days: number[] }) => r.days[0] === 3).ranges).toHaveLength(2);
  });

  it("warns that no range keeps the profile locked", async () => {
    api.get.mockResolvedValue(state({ enabled: true, mode: "week", rules: [{ days: [1, 2, 3, 4, 5], ranges: [{ start: 1080, end: 1320 }] }] }));
    renderDialog();
    await screen.findByText("Monday to Friday");
    const weekdays = screen.getByText("Monday to Friday").closest("div")!.parentElement!;
    fireEvent.click(within(weekdays).getByRole("button", { name: "Remove this time range" }));
    expect(screen.getByText("No time range: the profile stays locked at all times.")).toBeInTheDocument();
  });
});
