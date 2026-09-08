import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PlaybackSettings } from "./PlaybackSettings";

const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);
let mockProfiles: unknown[] = [];

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return { ...actual, useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }) };
});

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: { id: "u1" } }),
  useUserServiceListProfiles: () => ({ data: { profiles: mockProfiles } }),
  useUserServiceUpdateProfile: () => ({ mutateAsync: mockMutateAsync }),
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));

// The active profile is the second one: these preferences drive playback for
// whoever is watching, so writing them onto the household default would change
// the rules for somebody else.
vi.mock("@evey/store", async (importActual) => {
  const actual = await importActual<typeof import("@evey/store")>();
  return { ...actual, useAuthStore: (sel: (s: { profileId: string }) => unknown) => sel({ profileId: "p2" }) };
});

function renderIt() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return render(
    <QueryClientProvider client={qc}><PlaybackSettings /></QueryClientProvider>,
  );
}

describe("PlaybackSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockProfiles = [
      { id: "p1", isDefault: true, preferences: {} },
      { id: "p2", isDefault: false, preferences: { audio_languages: "fr" } },
    ];
  });

  it("offers the same playback preferences the mobile and TV apps have", () => {
    renderIt();
    for (const label of ["Max Streaming Bitrate", "Preferred Video Codec", "Preferred Audio Codec", "Max Audio Channels", "Subtitle Burn-in"]) {
      expect(screen.getByLabelText(label)).toBeInTheDocument();
    }
    expect(screen.getByTestId("playback-toggle-allow_direct_play")).toBeInTheDocument();
    expect(screen.getByTestId("playback-toggle-allow_direct_stream")).toBeInTheDocument();
    expect(screen.getByTestId("playback-toggle-allow_transcoding")).toBeInTheDocument();
  });

  it("reads the values off the ACTIVE profile", () => {
    mockProfiles = [
      { id: "p1", isDefault: true, preferences: { max_bitrate_kbps: "40000" } },
      { id: "p2", isDefault: false, preferences: { max_bitrate_kbps: "4000", subtitle_burn_in: "never" } },
    ];
    renderIt();
    expect((screen.getByLabelText("Max Streaming Bitrate") as HTMLSelectElement).value).toBe("4000");
    expect((screen.getByLabelText("Subtitle Burn-in") as HTMLSelectElement).value).toBe("never");
  });

  it("defaults to unlimited and auto when the profile never chose", () => {
    renderIt();
    expect((screen.getByLabelText("Max Streaming Bitrate") as HTMLSelectElement).value).toBe("0");
    expect((screen.getByLabelText("Preferred Video Codec") as HTMLSelectElement).value).toBe("auto");
    expect(screen.getByTestId("playback-toggle-allow_direct_play")).toHaveAttribute("aria-checked", "true");
  });

  it("merges the change onto the profile's other preferences", async () => {
    const user = userEvent.setup();
    renderIt();
    await user.selectOptions(screen.getByLabelText("Max Streaming Bitrate"), "8000");
    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(1));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      id: "p2",
      data: { preferences: { audio_languages: "fr", max_bitrate_kbps: "8000" } },
    });
    expect(mockInvalidateQueries).toHaveBeenCalledWith({ queryKey: ["profiles", "u1"] });
  });

  it("writes 'false' when a transcoding toggle is turned off", async () => {
    const user = userEvent.setup();
    renderIt();
    await user.click(screen.getByTestId("playback-toggle-allow_transcoding"));
    await waitFor(() => expect(mockMutateAsync).toHaveBeenCalledTimes(1));
    expect(mockMutateAsync).toHaveBeenCalledWith({
      id: "p2",
      data: { preferences: { audio_languages: "fr", allow_transcoding: "false" } },
    });
  });
});
