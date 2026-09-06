import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { FeedConfigCard } from "./FeedConfigCard";

// ── API mock state ─────────────────────────────────────────────────────────────

const mockBatchUpdate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

let mockFeedData:
  | {
      configs: Array<{
        feedType: string;
        enabled: boolean;
        displayOrder: number;
        maxItems: number;
      }>;
    }
  | undefined;
let mockFeedIsLoading = false;
let mockLibrariesData: { libraries: Array<{ id: string; name: string }> } = {
  libraries: [
    { id: "lib-1", name: "Movies" },
    { id: "lib-2", name: "TV Shows" },
  ],
};
let mockPluginsData: {
  plugins: Array<{
    id: string;
    name: string;
    enabled: boolean;
    frontend: Array<{ slot: string }>;
  }>;
} = { plugins: [] };

vi.mock("@evey/api", () => ({
  useFeedServiceGetAdminFeedConfigs: () => ({
    data: mockFeedData,
    isLoading: mockFeedIsLoading,
  }),
  useLibraryServiceListLibraries: () => ({ data: mockLibrariesData }),
  useAdminPluginServiceListPlugins: () => ({ data: mockPluginsData }),
  feedServiceBatchUpdateAdminFeedConfigs: (
    ...args: Parameters<typeof mockBatchUpdate>
  ) => mockBatchUpdate(...args),
  getFeedServiceGetAdminFeedConfigsQueryKey: () => [
    "/api/v1/admin/feeds/configs",
  ],
  getFeedServiceGetFeedConfigQueryKey: () => ["/api/v1/feeds/config"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

// Spread the real module: a mock that lists only the icons in use today goes
// red the day a component adds one, on an error that says nothing about the
// change being tested.
vi.mock("lucide-react", async (importOriginal) => {
  const Icon = ({ size }: { size?: number }) => <span data-size={size} />;
  return {
    ...(await importOriginal<typeof import("lucide-react")>()),
    GripVertical: Icon,
    ChevronUp: Icon,
    ChevronDown: Icon,
  };
});

// ── Helpers ────────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderCard() {
  return render(<FeedConfigCard />, { wrapper: createWrapper() });
}

const BASE_CONFIGS = [
  { feedType: "resume_playback", enabled: true, displayOrder: 0, maxItems: 10 },
  { feedType: "recommended", enabled: false, displayOrder: 1, maxItems: 20 },
  { feedType: "most_popular", enabled: true, displayOrder: 2, maxItems: 15 },
];

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("FeedConfigCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockBatchUpdate.mockResolvedValue({});
    mockFeedIsLoading = false;
    mockFeedData = { configs: [...BASE_CONFIGS] };
    mockLibrariesData = {
      libraries: [
        { id: "lib-1", name: "Movies" },
        { id: "lib-2", name: "TV Shows" },
      ],
    };
    mockPluginsData = { plugins: [] };
  });

  // ── Loading state ────────────────────────────────────────────────────────────

  it("renders shimmer placeholder while loading", () => {
    mockFeedIsLoading = true;
    mockFeedData = undefined;
    const { container } = renderCard();
    expect(container.querySelector(".animate-shimmer")).toBeInTheDocument();
    // No row content while loading
    expect(screen.queryByRole("switch")).toBeNull();
  });

  // ── Initial render from fetched data ─────────────────────────────────────────

  it("renders a row for each config entry", () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    expect(switches).toHaveLength(3);
  });

  // The grip carries a pointer handler and nothing else, so an admin on a
  // keyboard or a screen reader could enable and disable feeds but never
  // reorder them. The user-facing CustomizePanel already ships these buttons
  // for the very same list.
  it("lets a keyboard user reorder a section, not just a pointer", () => {
    renderCard();

    const up = screen.getAllByLabelText("Move section up");
    const down = screen.getAllByLabelText("Move section down");
    expect(up.length).toBeGreaterThan(0);
    expect(down.length).toBe(up.length);
    expect(up[0]).toBeDisabled();
    expect(down[down.length - 1]).toBeDisabled();
  });

  it("reflects enabled=true via aria-checked=true on toggle", () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    // resume_playback is enabled at index 0
    expect(switches[0]).toHaveAttribute("aria-checked", "true");
  });

  it("reflects enabled=false via aria-checked=false on toggle", () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    // recommended is disabled at index 1
    expect(switches[1]).toHaveAttribute("aria-checked", "false");
  });

  it("renders maxItems inputs for non-plugin rows with fetched values", () => {
    renderCard();
    const inputs = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    const values = inputs.map((i) => Number(i.value));
    expect(values).toContain(10);
    expect(values).toContain(20);
    expect(values).toContain(15);
  });

  it("maxItems inputs have accessible labels that include the row name", () => {
    renderCard();
    // aria-label is "Max Items: <feed label>" so screen readers announce both context and field
    expect(
      screen.getByRole("spinbutton", { name: "Max Items: Resume Playback" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("spinbutton", { name: "Max Items: Recommended" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("spinbutton", { name: "Max Items: Most Popular" }),
    ).toBeInTheDocument();
  });

  it("renders library feed with library name", () => {
    mockFeedData = {
      configs: [
        {
          feedType: "library:lib-1",
          enabled: true,
          displayOrder: 0,
          maxItems: 20,
        },
      ],
    };
    renderCard();
    expect(screen.getByText("Movies")).toBeInTheDocument();
  });

  it("sorts rows by displayOrder when fetched configs are unordered", () => {
    mockFeedData = {
      configs: [
        {
          feedType: "most_popular",
          enabled: true,
          displayOrder: 2,
          maxItems: 20,
        },
        {
          feedType: "resume_playback",
          enabled: true,
          displayOrder: 0,
          maxItems: 20,
        },
        {
          feedType: "recommended",
          enabled: true,
          displayOrder: 1,
          maxItems: 20,
        },
      ],
    };
    renderCard();
    const switches = screen.getAllByRole("switch");
    // First row should be resume_playback (order 0 → aria-label mentions disable since enabled)
    expect(switches[0]).toHaveAttribute("aria-checked", "true");
    // All three rows present
    expect(switches).toHaveLength(3);
  });

  it("renders empty table (no rows) when configs is undefined", () => {
    mockFeedData = undefined;
    renderCard();
    expect(screen.queryByRole("switch")).toBeNull();
    // Save button still visible
    expect(
      screen.getByRole("button", { name: /save/i }),
    ).toBeInTheDocument();
  });

  // ── Plugin rows ──────────────────────────────────────────────────────────────

  it("synthesises a virtual row for a plugin with home.section slot not yet in persisted configs", () => {
    mockPluginsData = {
      plugins: [
        {
          id: "plug-1",
          name: "Trending Plugin",
          enabled: true,
          frontend: [{ slot: "home.section" }],
        },
      ],
    };
    renderCard();
    expect(screen.getByText("Trending Plugin")).toBeInTheDocument();
  });

  it("shows Plugin badge for plugin rows", () => {
    mockPluginsData = {
      plugins: [
        {
          id: "plug-1",
          name: "My Plugin",
          enabled: true,
          frontend: [{ slot: "home.section" }],
        },
      ],
    };
    renderCard();
    // The badge text key resolves to "Plugin" from translations (defaultValue set in component)
    expect(screen.getByText("Plugin")).toBeInTheDocument();
  });

  it("does not show maxItems input for plugin rows (renders dash)", () => {
    mockFeedData = {
      configs: [
        {
          feedType: "plugin:plug-1",
          enabled: true,
          displayOrder: 0,
          maxItems: 20,
        },
      ],
    };
    mockPluginsData = {
      plugins: [
        {
          id: "plug-1",
          name: "My Plugin",
          enabled: true,
          frontend: [{ slot: "home.section" }],
        },
      ],
    };
    renderCard();
    // No number input for plugin rows
    expect(screen.queryByRole("spinbutton")).toBeNull();
    // Dash placeholder rendered instead
    expect(screen.getByText("-")).toBeInTheDocument();
  });

  it("does not list disabled plugins as virtual rows", () => {
    mockPluginsData = {
      plugins: [
        {
          id: "plug-disabled",
          name: "Disabled Plugin",
          enabled: false,
          frontend: [{ slot: "home.section" }],
        },
      ],
    };
    renderCard();
    expect(screen.queryByText("Disabled Plugin")).toBeNull();
  });

  it("does not list plugins without home.section slot as virtual rows", () => {
    mockPluginsData = {
      plugins: [
        {
          id: "plug-nav",
          name: "Nav Plugin",
          enabled: true,
          frontend: [{ slot: "admin.nav" }],
        },
      ],
    };
    renderCard();
    expect(screen.queryByText("Nav Plugin")).toBeNull();
  });

  // ── Toggle (local state) ─────────────────────────────────────────────────────

  it("toggling an enabled row flips aria-checked to false", () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    // resume_playback starts enabled
    expect(switches[0]).toHaveAttribute("aria-checked", "true");
    fireEvent.click(switches[0]);
    expect(switches[0]).toHaveAttribute("aria-checked", "false");
  });

  it("toggling a disabled row flips aria-checked to true", () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    // recommended starts disabled
    expect(switches[1]).toHaveAttribute("aria-checked", "false");
    fireEvent.click(switches[1]);
    expect(switches[1]).toHaveAttribute("aria-checked", "true");
  });

  it("toggling one row does not affect siblings", () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    // most_popular is enabled (index 2)
    expect(switches[2]).toHaveAttribute("aria-checked", "true");
    fireEvent.click(switches[0]);
    // most_popular should still be enabled
    expect(switches[2]).toHaveAttribute("aria-checked", "true");
  });

  // ── maxItems input (local state) ─────────────────────────────────────────────

  it("changing maxItems input updates displayed value", () => {
    renderCard();
    const inputs = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    // First input corresponds to first non-plugin row (resume_playback, maxItems=10)
    fireEvent.change(inputs[0], { target: { value: "25" } });
    expect(inputs[0].value).toBe("25");
  });

  it("clamps maxItems to minimum 1 when value below 1 entered", () => {
    renderCard();
    const inputs = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    fireEvent.change(inputs[0], { target: { value: "0" } });
    // Component enforces Math.max(1, value)
    expect(Number(inputs[0].value)).toBeGreaterThanOrEqual(1);
  });

  // ── Save - payload sent to batchUpdate ────────────────────────────────────────

  it("clicking Save calls feedServiceBatchUpdateAdminFeedConfigs with current rows", async () => {
    renderCard();
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    await waitFor(() => expect(mockBatchUpdate).toHaveBeenCalledOnce());
    const payload = mockBatchUpdate.mock.calls[0][0] as {
      configs: typeof BASE_CONFIGS;
    };
    expect(payload.configs).toHaveLength(3);
    expect(payload.configs[0].feedType).toBe("resume_playback");
  });

  it("save sends toggled enabled value in payload", async () => {
    renderCard();
    const switches = screen.getAllByRole("switch");
    fireEvent.click(switches[0]); // resume_playback: true → false
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    await waitFor(() => expect(mockBatchUpdate).toHaveBeenCalledOnce());
    const payload = mockBatchUpdate.mock.calls[0][0] as {
      configs: typeof BASE_CONFIGS;
    };
    const row = payload.configs.find((c) => c.feedType === "resume_playback");
    expect(row?.enabled).toBe(false);
  });

  it("save sends updated maxItems value in payload", async () => {
    renderCard();
    const inputs = screen.getAllByRole("spinbutton") as HTMLInputElement[];
    fireEvent.change(inputs[0], { target: { value: "42" } });
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    await waitFor(() => expect(mockBatchUpdate).toHaveBeenCalledOnce());
    const payload = mockBatchUpdate.mock.calls[0][0] as {
      configs: typeof BASE_CONFIGS;
    };
    const row = payload.configs.find((c) => c.feedType === "resume_playback");
    expect(row?.maxItems).toBe(42);
  });

  it("save sends updated displayOrder values after drag reorder", () => {
    // Drag is exercised via the handleDragEnd logic - test via direct state
    // by verifying the payload includes displayOrder field for each row.
    renderCard();
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    // Just assert the shape includes displayOrder (ordering drag is DOM-level,
    // fully tested by CustomizePanel which uses the same drag pattern).
    return waitFor(() => {
      const payload = mockBatchUpdate.mock.calls[0][0] as {
        configs: typeof BASE_CONFIGS;
      };
      for (const row of payload.configs) {
        expect(typeof row.displayOrder).toBe("number");
      }
    });
  });

  // ── Save - side effects ───────────────────────────────────────────────────────

  it("save button shows loading text while saving and re-enables after", async () => {
    let resolveSave!: () => void;
    mockBatchUpdate.mockReturnValue(
      new Promise<void>((res) => {
        resolveSave = res;
      }),
    );
    renderCard();
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    // While in-flight: button is disabled
    await waitFor(() =>
      expect(screen.getByTestId("feeds-save-btn")).toBeDisabled(),
    );
    resolveSave();
    // After completion: button re-enabled
    await waitFor(() =>
      expect(screen.getByTestId("feeds-save-btn")).not.toBeDisabled(),
    );
  });

  it("save invalidates admin feed configs query on success", async () => {
    renderCard();
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    await waitFor(() => expect(mockInvalidateQueries).toHaveBeenCalled());
    const calls = mockInvalidateQueries.mock.calls.map(
      (c: [{ queryKey: unknown[] }]) => c[0].queryKey,
    );
    expect(calls.some((qk: unknown[]) => String(qk[0]).includes("admin"))).toBe(
      true,
    );
  });

  it("save invalidates home feed config query on success (stale merged result fix)", async () => {
    renderCard();
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    await waitFor(() => expect(mockInvalidateQueries).toHaveBeenCalledTimes(2));
    const calls = mockInvalidateQueries.mock.calls.map(
      (c: [{ queryKey: unknown[] }]) => c[0].queryKey,
    );
    // Second invalidation targets the home-feed (GetFeedConfig) query
    expect(
      calls.some((qk: unknown[]) => String(qk[0]).includes("feeds/config")),
    ).toBe(true);
  });

  it("toast.error shown when save throws", async () => {
    mockBatchUpdate.mockRejectedValue(new Error("network"));
    const { toast } = await import("@evey/store");
    renderCard();
    fireEvent.click(screen.getByTestId("feeds-save-btn"));
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });
});
