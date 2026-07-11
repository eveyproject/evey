import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { PluginResourceLimits } from "./PluginResourceLimits";

// ── Mutable mock state ────────────────────────────────────────────────────────

const mockUpdateMutate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

let mockLimitsData:
  | {
      memoryMb?: number;
      pidsMax?: number;
      diskMaxMb?: number;
      cpuNice?: number;
      sandboxMode?: string;
    }
  | undefined;
let mockLimitsIsLoading = false;
let mockUpdateIsPending = false;

// Capture mutation callbacks so tests can fire them directly
let updateOnSuccess: (() => void) | undefined;
let updateOnError: (() => void) | undefined;

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@evey/api", () => ({
  useAdminPluginServiceGetPluginResourceLimits: () => ({
    data: mockLimitsData,
    isLoading: mockLimitsIsLoading,
  }),
  useAdminPluginServiceUpdatePluginResourceLimits: (opts: {
    mutation?: { onSuccess?: () => void; onError?: () => void };
  }) => {
    updateOnSuccess = opts?.mutation?.onSuccess;
    updateOnError = opts?.mutation?.onError;
    return { mutate: mockUpdateMutate, isPending: mockUpdateIsPending };
  },
  getAdminPluginServiceGetPluginResourceLimitsQueryKey: () => [
    "plugin-resource-limits",
  ],
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

// lucide-react is used by the Select component (ChevronDown icon)
vi.mock("lucide-react", () => {
  const Icon = ({ size, ...props }: { size?: number; [k: string]: unknown }) => (
    <span {...props} />
  );
  return { ChevronDown: Icon };
});

// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderCard() {
  return render(<PluginResourceLimits />, { wrapper: createWrapper() });
}

// ── Fixtures ──────────────────────────────────────────────────────────────────

const BASE_LIMITS = {
  memoryMb: 512,
  pidsMax: 64,
  diskMaxMb: 1024,
  cpuNice: 10,
  sandboxMode: "landlock",
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("PluginResourceLimits", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockLimitsIsLoading = false;
    mockUpdateIsPending = false;
    mockLimitsData = { ...BASE_LIMITS };
    updateOnSuccess = undefined;
    updateOnError = undefined;
    mockInvalidateQueries.mockResolvedValue(undefined);
  });

  // ── Loading state ─────────────────────────────────────────────────────────

  it("renders Spinner and no inputs while loading", () => {
    mockLimitsIsLoading = true;
    mockLimitsData = undefined;
    renderCard();
    // Spinner is rendered; numeric inputs are absent while loading
    expect(screen.queryByTestId("rl-memory")).toBeNull();
    expect(screen.queryByTestId("rl-save")).toBeNull();
  });

  it("renders inputs after loading completes", () => {
    renderCard();
    expect(screen.getByTestId("rl-memory")).toBeInTheDocument();
    expect(screen.getByTestId("rl-pids")).toBeInTheDocument();
    expect(screen.getByTestId("rl-disk")).toBeInTheDocument();
    expect(screen.getByTestId("rl-nice")).toBeInTheDocument();
    expect(screen.getByTestId("rl-sandbox")).toBeInTheDocument();
    expect(screen.getByTestId("rl-save")).toBeInTheDocument();
  });

  // ── Hydration from fetched data ───────────────────────────────────────────

  it("hydrates memoryMb input from fetched data", () => {
    renderCard();
    const input = screen.getByTestId("rl-memory") as HTMLInputElement;
    expect(Number(input.value)).toBe(512);
  });

  it("hydrates pidsMax input from fetched data", () => {
    renderCard();
    const input = screen.getByTestId("rl-pids") as HTMLInputElement;
    expect(Number(input.value)).toBe(64);
  });

  it("hydrates diskMaxMb input from fetched data", () => {
    renderCard();
    const input = screen.getByTestId("rl-disk") as HTMLInputElement;
    expect(Number(input.value)).toBe(1024);
  });

  it("hydrates cpuNice input from fetched data", () => {
    renderCard();
    const input = screen.getByTestId("rl-nice") as HTMLInputElement;
    expect(Number(input.value)).toBe(10);
  });

  it("hydrates sandboxMode select from fetched data", () => {
    renderCard();
    const select = screen.getByTestId("rl-sandbox") as HTMLSelectElement;
    expect(select.value).toBe("landlock");
  });

  it("falls back to 0 / 'auto' when data fields are absent", () => {
    mockLimitsData = {};
    renderCard();
    expect(Number((screen.getByTestId("rl-memory") as HTMLInputElement).value)).toBe(0);
    expect(Number((screen.getByTestId("rl-pids") as HTMLInputElement).value)).toBe(0);
    expect(Number((screen.getByTestId("rl-disk") as HTMLInputElement).value)).toBe(0);
    expect(Number((screen.getByTestId("rl-nice") as HTMLInputElement).value)).toBe(0);
    expect((screen.getByTestId("rl-sandbox") as HTMLSelectElement).value).toBe("auto");
  });

  // ── Local state updates ───────────────────────────────────────────────────

  it("changing memoryMb input updates displayed value", () => {
    renderCard();
    const input = screen.getByTestId("rl-memory") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "256" } });
    expect(Number(input.value)).toBe(256);
  });

  it("changing pidsMax input updates displayed value", () => {
    renderCard();
    const input = screen.getByTestId("rl-pids") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "32" } });
    expect(Number(input.value)).toBe(32);
  });

  it("changing diskMaxMb input updates displayed value", () => {
    renderCard();
    const input = screen.getByTestId("rl-disk") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "2048" } });
    expect(Number(input.value)).toBe(2048);
  });

  it("changing cpuNice input updates displayed value", () => {
    renderCard();
    const input = screen.getByTestId("rl-nice") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "15" } });
    expect(Number(input.value)).toBe(15);
  });

  it("changing sandboxMode select updates displayed value", () => {
    renderCard();
    const select = screen.getByTestId("rl-sandbox") as HTMLSelectElement;
    // The Select component calls onChange(e.target.value) not onChange(e)
    fireEvent.change(select, { target: { value: "bubblewrap" } });
    expect(select.value).toBe("bubblewrap");
  });

  it("cpuNice input has min=0 and max=19 attributes", () => {
    renderCard();
    const input = screen.getByTestId("rl-nice") as HTMLInputElement;
    expect(input.min).toBe("0");
    expect(input.max).toBe("19");
  });

  it("sandboxMode select renders all five mode options", () => {
    renderCard();
    const select = screen.getByTestId("rl-sandbox") as HTMLSelectElement;
    const options = Array.from(select.options).map((o) => o.value);
    expect(options).toContain("auto");
    expect(options).toContain("landlock");
    expect(options).toContain("bubblewrap");
    expect(options).toContain("strict");
    expect(options).toContain("basic");
  });

  // ── Save - payload ────────────────────────────────────────────────────────

  it("clicking save calls update.mutate with the current field values", () => {
    renderCard();
    fireEvent.click(screen.getByTestId("rl-save"));
    expect(mockUpdateMutate).toHaveBeenCalledOnce();
    expect(mockUpdateMutate).toHaveBeenCalledWith({
      data: {
        memoryMb: 512,
        pidsMax: 64,
        diskMaxMb: 1024,
        cpuNice: 10,
        sandboxMode: "landlock",
      },
    });
  });

  it("save sends edited numeric values in payload", () => {
    renderCard();
    fireEvent.change(screen.getByTestId("rl-memory"), { target: { value: "768" } });
    fireEvent.change(screen.getByTestId("rl-pids"), { target: { value: "128" } });
    fireEvent.click(screen.getByTestId("rl-save"));
    expect(mockUpdateMutate).toHaveBeenCalledWith({
      data: expect.objectContaining({ memoryMb: 768, pidsMax: 128 }),
    });
  });

  it("save sends edited sandboxMode in payload", () => {
    renderCard();
    fireEvent.change(screen.getByTestId("rl-sandbox"), { target: { value: "strict" } });
    fireEvent.click(screen.getByTestId("rl-save"));
    expect(mockUpdateMutate).toHaveBeenCalledWith({
      data: expect.objectContaining({ sandboxMode: "strict" }),
    });
  });

  // ── Save button loading state ─────────────────────────────────────────────

  it("save button is disabled while update.isPending", () => {
    mockUpdateIsPending = true;
    renderCard();
    expect(screen.getByTestId("rl-save")).toBeDisabled();
  });

  it("save button is enabled when update is not pending", () => {
    renderCard();
    expect(screen.getByTestId("rl-save")).not.toBeDisabled();
  });

  // ── Save - side effects ───────────────────────────────────────────────────

  it("onSuccess invalidates the resource limits query key", async () => {
    renderCard();
    fireEvent.click(screen.getByTestId("rl-save"));
    updateOnSuccess?.();
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["plugin-resource-limits"] }),
      ),
    );
  });

  it("onSuccess shows success toast", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    fireEvent.click(screen.getByTestId("rl-save"));
    updateOnSuccess?.();
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  it("onError shows error toast", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    fireEvent.click(screen.getByTestId("rl-save"));
    updateOnError?.();
    await waitFor(() => expect(toast.error).toHaveBeenCalled());
  });
});
