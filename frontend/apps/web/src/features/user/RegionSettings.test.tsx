import React from "react";
import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { RegionSettings } from "./RegionSettings";

// ── Mutable mock state ────────────────────────────────────────────────────────

const mockMutateAsync = vi.fn().mockResolvedValue(undefined);
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

let mockCurrentUser: unknown = { id: "u1", username: "alice" };
let mockProfiles: unknown[] = [{ id: "p1", isDefault: true, region: "US" }];

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", () => ({
  useUserServiceGetCurrentUser: () => ({ data: mockCurrentUser }),
  useUserServiceListProfiles: (_id: string, _opts?: unknown) => ({
    data: { profiles: mockProfiles },
  }),
  useUserServiceUpdateProfile: () => ({ mutateAsync: mockMutateAsync }),
  getUserServiceListProfilesQueryKey: (id: string) => ["profiles", id],
}));

// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderRegionSettings() {
  return render(<RegionSettings />, { wrapper: createWrapper() });
}

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("RegionSettings", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCurrentUser = { id: "u1", username: "alice" };
    mockProfiles = [{ id: "p1", isDefault: true, region: "US" }];
    mockMutateAsync.mockResolvedValue(undefined);
  });

  // ── Heading rendering ─────────────────────────────────────────────────────────

  it("renders region heading", () => {
    renderRegionSettings();
    // i18n key common.region.title - resolves to "Region" in tests
    expect(screen.getAllByText(/common\.region\.title|Region/i)[0]).toBeInTheDocument();
  });

  it("renders region description", () => {
    renderRegionSettings();
    // i18n key resolves to literal key in test environment
    expect(screen.getByText("Used for content classifications and regional preferences.")).toBeInTheDocument();
  });

  // ── Select rendered ───────────────────────────────────────────────────────────

  it("renders a combobox (select)", () => {
    renderRegionSettings();
    expect(screen.getByRole("combobox")).toBeInTheDocument();
  });

  // ── Current value hydrates from profile region ─────────────────────────────

  it("select value reflects the default profile region (US)", () => {
    renderRegionSettings();
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    expect(select.value).toBe("US");
  });

  it("select value is empty string when profile has no region", () => {
    mockProfiles = [{ id: "p1", isDefault: true, region: "" }];
    renderRegionSettings();
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    expect(select.value).toBe("");
  });

  it("uses the first profile when none is marked isDefault", () => {
    mockProfiles = [{ id: "p2", isDefault: false, region: "DE" }];
    renderRegionSettings();
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    expect(select.value).toBe("DE");
  });

  // ── Auto option exists ────────────────────────────────────────────────────────

  it("includes an Auto option with empty value", () => {
    renderRegionSettings();
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.options);
    const autoOption = options.find((o) => o.value === "");
    expect(autoOption).toBeDefined();
  });

  // ── Region options are present ────────────────────────────────────────────────

  it("includes US as a selectable option", () => {
    renderRegionSettings();
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.options);
    const usOption = options.find((o) => o.value === "US");
    expect(usOption).toBeDefined();
  });

  it("includes FR as a selectable option", () => {
    renderRegionSettings();
    const select = screen.getByRole("combobox") as HTMLSelectElement;
    const options = Array.from(select.options);
    const frOption = options.find((o) => o.value === "FR");
    expect(frOption).toBeDefined();
  });

  // ── Changing selection calls mutation ─────────────────────────────────────────

  it("selecting a new region calls mutateAsync with the profile id and region code", async () => {
    const user = userEvent.setup();
    renderRegionSettings();
    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "DE");
    await waitFor(() =>
      expect(mockMutateAsync).toHaveBeenCalledWith({ id: "p1", data: { region: "DE" } }),
    );
  });

  it("selecting Auto (empty string) saves empty region", async () => {
    const user = userEvent.setup();
    mockProfiles = [{ id: "p1", isDefault: true, region: "US" }];
    renderRegionSettings();
    const select = screen.getByRole("combobox");
    // select the option with value ""
    await user.selectOptions(select, "");
    await waitFor(() =>
      expect(mockMutateAsync).toHaveBeenCalledWith({ id: "p1", data: { region: "" } }),
    );
  });

  // ── Query invalidation on success ─────────────────────────────────────────────

  it("invalidates profiles query after a successful update", async () => {
    const user = userEvent.setup();
    renderRegionSettings();
    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "GB");
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["profiles", "u1"] }),
      ),
    );
  });

  // ── No user id: mutation NOT called ──────────────────────────────────────────

  it("does NOT call mutateAsync when there is no profile id", async () => {
    const user = userEvent.setup();
    mockProfiles = [];
    renderRegionSettings();
    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "FR");
    // handleChange returns early when defaultProfile?.id is falsy
    expect(mockMutateAsync).not.toHaveBeenCalled();
  });

  // ── Error resilience (no throw on mutateAsync rejection) ─────────────────────

  it("does not throw when mutateAsync rejects (silent console.warn)", async () => {
    const user = userEvent.setup();
    mockMutateAsync.mockRejectedValueOnce(new Error("network error"));
    const warnSpy = vi.spyOn(console, "warn").mockImplementation(() => {});
    renderRegionSettings();
    const select = screen.getByRole("combobox");
    await user.selectOptions(select, "CA");
    await waitFor(() => expect(warnSpy).toHaveBeenCalledWith("region update failed", expect.any(Error)));
    warnSpy.mockRestore();
  });
});
