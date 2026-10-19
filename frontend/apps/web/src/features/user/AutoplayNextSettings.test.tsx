import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { AutoplayNextSettings } from "./AutoplayNextSettings";

const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);
let mockProfiles: unknown[] = [];

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }) };
});

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1", username: "alice" } }),
  useUserServiceListProfiles: () => ({ data: { profiles: mockProfiles } }),
  useUserServiceUpdateProfile: () => ({ mutateAsync: mockMutateAsync }),
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));

// The active profile is the second one: the switch must read and write THAT
// profile, never the default.
vi.mock("@evey/store", async (importActual) => {
  const actual = await importActual<typeof import("@evey/store")>();
  return { ...actual, useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: "p2" }) };
});

function renderIt() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}><AutoplayNextSettings /></QueryClientProvider>,
  );
}

describe("AutoplayNextSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockProfiles = [
      { id: "p1", isDefault: true, preferences: {} },
      { id: "p2", isDefault: false, preferences: { theme: "dark" } },
    ];
  });

  it("is on when the profile never chose", () => {
    renderIt();
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "true");
  });

  it("reflects an explicit off on the active profile", () => {
    mockProfiles = [
      { id: "p1", isDefault: true, preferences: {} },
      { id: "p2", isDefault: false, preferences: { autoplay_next_episode: "false" } },
    ];
    renderIt();
    expect(screen.getByRole("switch")).toHaveAttribute("aria-checked", "false");
  });

  it("writes the choice onto the active profile, merged with its other preferences", async () => {
    const user = userEvent.setup();
    renderIt();
    await user.click(screen.getByRole("switch"));
    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(1));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      id: "p2",
      data: { preferences: { theme: "dark", autoplay_next_episode: "false" } },
    });
    expect(mockInvalidateQueries).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] });
  });

  // Both controls merge into the cached preferences and the server replaces
  // the whole map: a second write before the first one's refetch dropped it.
  it("holds both controls until the written profile is fresh again", async () => {
    let refetched!: () => void;
    mockInvalidateQueries.mockImplementationOnce(() => new Promise<void>((r) => { refetched = r; }));
    const user = userEvent.setup();
    renderIt();
    await user.click(screen.getByRole("switch"));
    await waitFor(() => expect(mockInvalidateQueries).toHaveBeenCalledTimes(1));
    expect(screen.getByRole("switch")).toBeDisabled();
    expect(screen.getByTestId("still-watching-select")).toBeDisabled();
    refetched();
    await waitFor(() => expect(screen.getByRole("switch")).not.toBeDisabled());
    expect(screen.getByTestId("still-watching-select")).not.toBeDisabled();
  });
});
