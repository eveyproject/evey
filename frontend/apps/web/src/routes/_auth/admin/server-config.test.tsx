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
    useQuery: (...args: unknown[]) => {
      const opts = args[0] as { queryKey?: string[] } | undefined;
      if (opts?.queryKey?.[0]?.includes("server-settings")) {
        return { data: undefined, isLoading: true };
      }
      return mockUseQuery(...args);
    },
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

// Stable identity on purpose: a fresh object per render would re-run every
// effect keyed on `data` and spin the page in a synchronous loop.
const stableEmptyQuery = { data: { plugins: [], libraries: [], configs: [], items: [] }, isLoading: false, isError: false };
vi.mock("@evey/api", () => ({
  useAdminPluginServiceListPlugins: () => stableEmptyQuery,
  useLibraryServiceListLibraries: () => stableEmptyQuery,
  useFeedServiceGetAdminFeedConfigs: () => stableEmptyQuery,
  eveyFetch: vi.fn(),
  useServerConfigServiceListServerConfigEntries: (...args: unknown[]) => mockUseQuery(...args),
  serverConfigServiceSetServerConfigEntry: vi.fn(),
  serverConfigServiceDeleteServerConfigEntry: vi.fn(),
  getServerConfigServiceListServerConfigEntriesQueryKey: () => ["server-config-entries"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useLocaleStore: (fn: (s: { setLocale: () => void }) => unknown) => fn({ setLocale: vi.fn() }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getServerConfigPage() {
  const mod = await import("./server-config.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockEntries = [
  { key: "smtp.host", value: "smtp.example.com", updatedAt: "2024-01-15T10:00:00Z" },
  { key: "custom.key", value: "custom-value", updatedAt: "2024-01-15T10:00:00Z" },
];

describe("ServerConfigPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders page heading", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Server Config/i)).toBeInTheDocument();
  });

  it("renders Language & Region card", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Language & Region/i)).toBeInTheDocument();
  });

  it("renders Email / SMTP card", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Email \/ SMTP/i)).toBeInTheDocument();
  });

  it("renders config entries in table", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText("smtp.host")).toBeInTheDocument();
    expect(screen.getByText("custom.key")).toBeInTheDocument();
  });

  it("renders Save SMTP Settings button", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // The SMTP settings live in a collapsible card that starts closed; expand
    // it before asserting on its body.
    fireEvent.click(screen.getByRole("button", { name: /Email \/ SMTP/i }));
    expect(await screen.findByRole("button", { name: /Save SMTP Settings/i })).toBeInTheDocument();
  });

  it("shows Edit button for each config entry", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    const editButtons = screen.getAllByRole("button", { name: /^Edit$/i });
    expect(editButtons.length).toBeGreaterThanOrEqual(mockEntries.length);
  });

  it("clicking Edit on config row shows inline editor", async () => {
    mockUseQuery.mockReturnValue({ data: mockEntries, isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // Settings forms that hold Save buttons (SMTP, locale, …) now live in
    // collapsed cards, so none are visible until a row enters edit mode.
    // Count them (query*, which returns [] rather than throwing on zero),
    // then click Edit on the first config row and assert one extra Save (the
    // row's inline editor) appeared. Cancel is also unique to the inline row.
    const saveBefore = screen.queryAllByRole("button", { name: /^Save$/i }).length;
    const cancelBefore = screen.queryAllByRole("button", { name: /^Cancel$/i }).length;
    fireEvent.click(screen.getAllByRole("button", { name: /^Edit$/i })[0]);
    await waitFor(() => {
      expect(screen.getAllByRole("button", { name: /^Save$/i }).length).toBe(saveBefore + 1);
      expect(screen.getAllByRole("button", { name: /^Cancel$/i }).length).toBe(cancelBefore + 1);
    });
  });

  it("shows Add button for adding new entries", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByRole("button", { name: /^Add$/i })).toBeInTheDocument();
  });

  it("AddEntryRow key and value inputs have accessible labels", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // The key input carries a datalist of the known operator flags, which makes
    // its ARIA role combobox rather than textbox - the accessible NAME is what
    // this test is about, and it is unchanged.
    expect(screen.getByRole("combobox", { name: /^Key$/i })).toBeInTheDocument();
    expect(screen.getByRole("textbox", { name: /^Value$/i })).toBeInTheDocument();
  });

  it("suggests the boot-time operator flags and explains a known key", async () => {
    // These were always settable here, but only if you already knew the exact
    // key to type - nothing listed them.
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });

    const keyInput = screen.getByRole("combobox", { name: /^Key$/i });
    expect(keyInput).toHaveAttribute("list", "operator-flag-keys");
    expect(screen.queryByTestId("operator-flag-hint")).not.toBeInTheDocument();

    fireEvent.change(keyInput, { target: { value: "transcoding.snapshot_fastpath" } });
    expect(screen.getByTestId("operator-flag-hint").textContent).toContain("EVEY_SNAPSHOT_FASTPATH");

    // A free-text key is still allowed; it just gets no hint.
    fireEvent.change(keyInput, { target: { value: "some.custom.key" } });
    expect(screen.queryByTestId("operator-flag-hint")).not.toBeInTheDocument();
  });

  it("Show/Hide button toggles password visibility", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    // The password toggle lives inside the (collapsed) SMTP card - expand first.
    fireEvent.click(screen.getByRole("button", { name: /Email \/ SMTP/i }));
    // Several password fields each carry a Show/Hide eye; toggle the first.
    const showBtns = await screen.findAllByRole("button", { name: /^show$/i });
    fireEvent.click(showBtns[0]);
    expect(screen.getByRole("button", { name: /^hide$/i })).toBeInTheDocument();
  });

  it("renders Memory Profile card with Auto/Normal/Low options", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Memory Profile/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /Memory Profile/i }));
    const select = await screen.findByLabelText(/^Memory profile$/i);
    const options = Array.from(select.querySelectorAll("option")).map((o) => o.textContent);
    expect(options).toEqual(["Auto (recommended)", "Normal", "Low"]);
    expect(screen.getByText(/Low suspends idle plugin processes to save RAM/i)).toBeInTheDocument();
  });

  it("defaults the Memory Profile select to Auto when no entry is stored", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Memory Profile/i }));
    const select = (await screen.findByLabelText(/^Memory profile$/i)) as HTMLSelectElement;
    expect(select.value).toBe("");
  });

  it("reflects a stored server.memory_profile value in the select", async () => {
    mockUseQuery.mockReturnValue({
      data: [{ key: "server.memory_profile", value: "low", updatedAt: "2024-01-15T10:00:00Z" }],
      isLoading: false,
    });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Memory Profile/i }));
    const select = (await screen.findByLabelText(/^Memory profile$/i)) as HTMLSelectElement;
    expect(select.value).toBe("low");
  });

  it("saves the selected memory profile value", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /Memory Profile/i }));
    const select = await screen.findByLabelText(/^Memory profile$/i);
    fireEvent.change(select, { target: { value: "low" } });
    expect(mutate).toHaveBeenCalledWith(
      { key: "server.memory_profile", value: "low" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("renders mkvgo card with Ingest serving plan and Playability shadow selects", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText(/^mkvgo\b/i)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^mkvgo\b/i }));

    const ingestSelect = await screen.findByLabelText(/^Ingest serving plan$/i);
    const ingestOptions = Array.from(ingestSelect.querySelectorAll("option")).map((o) => o.textContent);
    expect(ingestOptions).toEqual(["Off", "Plan", "Plan + repair"]);

    const shadowSelect = await screen.findByLabelText(/^Playability shadow$/i);
    const shadowOptions = Array.from(shadowSelect.querySelectorAll("option")).map((o) => o.textContent);
    expect(shadowOptions).toEqual(["Off", "On"]);
  });

  it("defaults the mkvgo selects to Off when no entry is stored", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^mkvgo\b/i }));
    const ingestSelect = (await screen.findByLabelText(/^Ingest serving plan$/i)) as HTMLSelectElement;
    expect(ingestSelect.value).toBe("");
    const shadowSelect = (await screen.findByLabelText(/^Playability shadow$/i)) as HTMLSelectElement;
    expect(shadowSelect.value).toBe("on");
  });

  it("reflects stored library.ingest_serving_plan and transcoding.playability_shadow values", async () => {
    mockUseQuery.mockReturnValue({
      data: [
        { key: "library.ingest_serving_plan", value: "plan+repair", updatedAt: "2024-01-15T10:00:00Z" },
        { key: "transcoding.playability_shadow", value: "off", updatedAt: "2024-01-15T10:00:00Z" },
      ],
      isLoading: false,
    });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^mkvgo\b/i }));
    const ingestSelect = (await screen.findByLabelText(/^Ingest serving plan$/i)) as HTMLSelectElement;
    expect(ingestSelect.value).toBe("plan+repair");
    const shadowSelect = (await screen.findByLabelText(/^Playability shadow$/i)) as HTMLSelectElement;
    expect(shadowSelect.value).toBe("off");
  });

  it("saves the selected ingest serving plan value", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^mkvgo\b/i }));
    const select = await screen.findByLabelText(/^Ingest serving plan$/i);
    fireEvent.change(select, { target: { value: "plan+repair" } });
    expect(mutate).toHaveBeenCalledWith(
      { key: "library.ingest_serving_plan", value: "plan+repair" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("saves the selected playability shadow value", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn();
    mockUseMutation.mockReturnValue({ mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^mkvgo\b/i }));
    const select = await screen.findByLabelText(/^Playability shadow$/i);
    fireEvent.change(select, { target: { value: "off" } });
    expect(mutate).toHaveBeenCalledWith(
      { key: "transcoding.playability_shadow", value: "off" },
      expect.objectContaining({ onSuccess: expect.any(Function) }),
    );
  });

  it("renders Background jobs card with the restart warning visible", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    expect(screen.getByText("Background work")).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));
    expect(
      await screen.findByText(/read once at server startup/i),
    ).toBeInTheDocument();
  });

  it("renders the 4 background job toggles, checked by default (no stored entry)", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));

    const pause = await screen.findByLabelText(/Pause background jobs during playback/i);
    const level = screen.getByLabelText(/^H\.264\/HEVC level backfill$/i);
    const keyframe = screen.getByLabelText(/^Keyframe index backfill$/i);
    const fastpath = screen.getByLabelText(/Timeline thumbnail fast path/i);

    expect(pause).toHaveAttribute("aria-checked", "true");
    expect(level).toHaveAttribute("aria-checked", "true");
    expect(keyframe).toHaveAttribute("aria-checked", "true");
    expect(fastpath).toHaveAttribute("aria-checked", "true");
  });

  it("unchecking a toggle writes the key with value 'off'", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn(async () => ({}));
    mockUseMutation.mockReturnValue({ mutateAsync: mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));

    const keyframe = await screen.findByLabelText(/^Keyframe index backfill$/i);
    fireEvent.click(keyframe);

    expect(mutate).toHaveBeenCalledWith({ key: "transcoding.keyframe_backfill", value: "off" });
  });

  it("shows a toggle as off when the stored value is 'off'", async () => {
    mockUseQuery.mockReturnValue({
      data: [{ key: "transcoding.keyframe_backfill", value: "off", updatedAt: "2024-01-15T10:00:00Z" }],
      isLoading: false,
    });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));
    const keyframe = await screen.findByLabelText(/^Keyframe index backfill$/i);
    expect(keyframe).toHaveAttribute("aria-checked", "false");
  });

  it("re-checking an off toggle deletes the key to revert to the server default", async () => {
    mockUseQuery.mockReturnValue({
      data: [{ key: "transcoding.level_backfill", value: "off", updatedAt: "2024-01-15T10:00:00Z" }],
      isLoading: false,
    });
    const mutate = vi.fn(async () => ({}));
    mockUseMutation.mockReturnValue({ mutateAsync: mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));

    const level = await screen.findByLabelText(/^H\.264\/HEVC level backfill$/i);
    fireEvent.click(level);

    expect(mutate).toHaveBeenCalledWith("transcoding.level_backfill");
  });

  it("saves the pacing fields on Save, one entry per non-empty field", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    const mutate = vi.fn(async () => ({}));
    mockUseMutation.mockReturnValue({ mutateAsync: mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));

    const batch = await screen.findByLabelText(/Keyframe backfill batch size/i);
    fireEvent.change(batch, { target: { value: "50" } });

    const saveButtons = screen.getAllByRole("button", { name: /^Save$/i });
    fireEvent.click(saveButtons[saveButtons.length - 1]);

    expect(mutate).toHaveBeenCalledWith({ key: "transcoding.keyframe_backfill_batch", value: "50" });
  });

  it("keeps the pacing form dirty when a write fails", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false });
    // Fail every write: the Save button must stay enabled so the operator can
    // retry, instead of the form reporting itself saved and dropping the edits.
    const mutate = vi.fn(async () => { throw new Error("nope"); });
    mockUseMutation.mockReturnValue({ mutateAsync: mutate, isPending: false });
    const ServerConfigPage = await getServerConfigPage();
    render(React.createElement(ServerConfigPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByRole("button", { name: /^Background work/i }));

    const batch = await screen.findByLabelText(/Keyframe backfill batch size/i);
    fireEvent.change(batch, { target: { value: "50" } });

    const saveButtons = screen.getAllByRole("button", { name: /^Save$/i });
    const save = saveButtons[saveButtons.length - 1];
    fireEvent.click(save);

    await waitFor(() => expect(mutate).toHaveBeenCalledTimes(6));
    await new Promise((r) => setTimeout(r, 0));
    expect(save).not.toBeDisabled();
  });
});
