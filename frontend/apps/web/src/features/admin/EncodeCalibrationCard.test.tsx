import { act, render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { EncodeCalibrationCard, EncodeSettingsCard } from "./EncodeCalibrationCard";

// ── Mutable mock state ─────────────────────────────────────────────────────────

const mockRecalibrateMutate = vi.fn();
const mockSetModeMutate = vi.fn();
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);
const mockSetQueryData = vi.fn();

let mockCalibrationData: Record<string, unknown> | undefined;
let mockSettingsData: Record<string, unknown> | undefined;
let mockCalibrationIsLoading = false;
let mockRecalibrateIsPending = false;
let mockSetModeIsPending = false;

// Callbacks for triggering mutation outcomes in tests
let recalibrateOnSuccess: ((data: Record<string, unknown>) => void) | undefined;
let recalibrateOnError: ((err: Error) => void) | undefined;
let setModeOnSuccess: ((resp: unknown, mode: string) => void) | undefined;
let setModeOnError: ((err: Error) => void) | undefined;

// ── Module mocks ───────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQuery: (opts: { queryKey: unknown[]; queryFn: unknown }) => {
      const key = opts.queryKey as string[];
      if (key[0] === "admin" && key[1] === "transcode-calibration") {
        return { data: mockCalibrationData, isLoading: mockCalibrationIsLoading };
      }
      if (key[0] === "/api/v1/admin/server-settings") {
        return { data: mockSettingsData, isLoading: false };
      }
      return { data: undefined, isLoading: false };
    },
    useMutation: (opts: {
      mutationFn: (arg: unknown) => Promise<unknown>;
      onSuccess?: (...args: unknown[]) => void;
      onError?: (err: Error) => void;
    }) => {
      // Distinguish the two mutations by shape of mutationFn string
      const fnStr = String(opts.mutationFn);
      if (fnStr.includes("recalibrate")) {
        recalibrateOnSuccess = opts.onSuccess as typeof recalibrateOnSuccess;
        recalibrateOnError = opts.onError;
        return {
          mutate: mockRecalibrateMutate.mockImplementation(() => {
            // do nothing - tests call recalibrateOnSuccess/Error directly
          }),
          isPending: mockRecalibrateIsPending,
        };
      }
      // setMode mutation (PUT server-settings)
      setModeOnSuccess = opts.onSuccess as typeof setModeOnSuccess;
      setModeOnError = opts.onError;
      return {
        mutate: mockSetModeMutate.mockImplementation(() => {}),
        isPending: mockSetModeIsPending,
      };
    },
    useQueryClient: () => ({
      invalidateQueries: mockInvalidateQueries,
      setQueryData: mockSetQueryData,
    }),
  };
});

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
}));

// ── Helpers ────────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderCard() {
  // The settings (hardware, bitrate, AV1) and the diagnostics (measured
  // ceilings, recalibrate) are two cards on the page; render them together
  // as the page does.
  return render(
    <>
      <EncodeSettingsCard />
      <EncodeCalibrationCard />
    </>,
    { wrapper: createWrapper() },
  );
}

// ── Fixtures ───────────────────────────────────────────────────────────────────

const SW_ONLY_DATA = {
  software_max_height: 1080,
  software_hdr_max_height: 720,
  hw_max_height: 0,
  hw_backend: undefined,
  hw_codecs: [],
  hw_decode_codecs: [],
  hw_device: undefined,
  gpu_class: undefined,
  hw_mode: "off",
  detected_backends: [],
  is_wsl: false,
  benchmarked_at: "2024-03-10T14:30:00Z",
  benchmark_duration_ms: 4500,
  hw_fallbacks: [],
};

const HW_NVENC_DATA = {
  software_max_height: 1080,
  software_hdr_max_height: 720,
  hw_max_height: 2160,
  hw_backend: "nvenc",
  hw_codecs: ["h264", "hevc", "av1"],
  hw_decode_codecs: ["h264", "hevc"],
  hw_device: "/dev/nvidia0",
  gpu_class: "discrete",
  hw_mode: "nvenc",
  detected_backends: ["nvenc"],
  is_wsl: false,
  benchmarked_at: "2024-03-10T14:30:00Z",
  benchmark_duration_ms: 8200,
  hw_fallbacks: [],
};

const HW_VAAPI_DATA = {
  ...HW_NVENC_DATA,
  hw_max_height: 1080,
  hw_backend: "vaapi",
  hw_codecs: ["h264"],
  hw_decode_codecs: ["h264"],
  hw_device: "/dev/dri/renderD128",
  gpu_class: "integrated",
  hw_mode: "vaapi",
  detected_backends: ["vaapi"],
};

const HW_FALLBACKS_DATA = {
  ...HW_NVENC_DATA,
  hw_fallbacks: [
    { class: "hw_unsupported_option", codec: "hevc", at: "2024-03-09T10:00:00Z" },
    { class: "hw_init", codec: "h264", at: "2024-03-09T11:00:00Z" },
  ],
};

const WSL_DATA = {
  ...HW_NVENC_DATA,
  is_wsl: true,
  detected_backends: ["nvenc", "vaapi"],
};

// ── Tests ──────────────────────────────────────────────────────────────────────

describe("EncodeCalibrationCard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCalibrationIsLoading = false;
    mockRecalibrateIsPending = false;
    mockSetModeIsPending = false;
    mockCalibrationData = SW_ONLY_DATA;
    mockSettingsData = undefined;
    recalibrateOnSuccess = undefined;
    recalibrateOnError = undefined;
    setModeOnSuccess = undefined;
    setModeOnError = undefined;
    mockInvalidateQueries.mockResolvedValue(undefined);
  });

  // ── Loading / never-calibrated state ─────────────────────────────────────────

  it("renders shimmer placeholder while loading", () => {
    mockCalibrationIsLoading = true;
    mockCalibrationData = undefined;
    const { container } = renderCard();
    expect(container.querySelector(".animate-shimmer")).toBeInTheDocument();
    expect(screen.queryByTestId("recalibrate-btn")).toBeNull();
  });

  it("renders error state (not an endless shimmer) when data is missing and not loading", () => {
    // Previously this rendered an endless shimmer; now a finished-but-empty
    // query shows an explicit error/empty state (TriangleAlert) so the card
    // doesn't pretend to be perpetually loading.
    mockCalibrationIsLoading = false;
    mockCalibrationData = undefined;
    const { container } = renderCard();
    expect(container.querySelector(".animate-shimmer")).toBeNull();
    expect(container.querySelector(".lucide-triangle-alert")).toBeInTheDocument();
  });

  // ── Software-only host ────────────────────────────────────────────────────────

  it("renders software encode cap (1080p) for software-only host", () => {
    renderCard();
    expect(screen.getByText("1080p")).toBeInTheDocument();
  });

  it("renders software HDR cap row when software_hdr_max_height is set", () => {
    renderCard();
    expect(screen.getByText("720p")).toBeInTheDocument();
  });

  it("does not render software HDR cap row when software_hdr_max_height is absent", () => {
    mockCalibrationData = { ...SW_ONLY_DATA, software_hdr_max_height: undefined };
    renderCard();
    // 720p is the HDR cap - should not appear
    expect(screen.queryByText("720p")).toBeNull();
  });

  it("renders 'no GPU accelerator detected' text for software-only host", () => {
    renderCard();
    expect(screen.getByText("no GPU accelerator detected")).toBeInTheDocument();
  });

  it("renders benchmark duration (4.5s) for software-only host", () => {
    renderCard();
    expect(screen.getByText(/4\.5s/)).toBeInTheDocument();
  });

  // ── HW toggle (software-only host, hw_mode=off) ───────────────────────────────

  it("HW toggle is off (aria-checked=false) when hw_mode is 'off'", () => {
    renderCard();
    const toggle = screen.getByTestId("hw-enable-toggle");
    expect(toggle).toHaveAttribute("aria-checked", "false");
  });

  it("shows 'Software only (CPU)' hint when HW is disabled", () => {
    renderCard();
    expect(screen.getByText(/Software only/i)).toBeInTheDocument();
  });

  it("backend select is hidden when HW is disabled", () => {
    renderCard();
    expect(screen.queryByTestId("hw-mode-select")).toBeNull();
  });

  it("clicking the HW toggle calls setMode mutation with 'auto'", () => {
    renderCard();
    fireEvent.click(screen.getByTestId("hw-enable-toggle"));
    expect(mockSetModeMutate).toHaveBeenCalledWith("auto");
  });

  // ── HW-capable host (NVENC discrete GPU) ─────────────────────────────────────

  it("renders HW encode cap (2160p) for NVENC host", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByText("2160p")).toBeInTheDocument();
  });

  it("renders hw_backend label (nvenc) next to the HW cap", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    // The backend appears inline: "(nvenc, dedicated GPU)" - multiple elements
    // may contain "nvenc" (select options, detected label), so assert at least one exists.
    const matches = screen.getAllByText(/nvenc/i);
    expect(matches.length).toBeGreaterThanOrEqual(1);
  });

  it("renders 'dedicated GPU' label for gpu_class=discrete", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByText(/dedicated GPU/i)).toBeInTheDocument();
  });

  it("renders hw_device path in the codec row", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByText(/\/dev\/nvidia0/)).toBeInTheDocument();
  });

  it("HW toggle is on (aria-checked=true) when hw_mode is a backend name", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByTestId("hw-enable-toggle")).toHaveAttribute("aria-checked", "true");
  });

  it("shows backend select when HW is enabled", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByTestId("hw-mode-select")).toBeInTheDocument();
  });

  it("backend select shows current hwMode value", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    const select = screen.getByTestId("hw-mode-select") as HTMLSelectElement;
    expect(select.value).toBe("nvenc");
  });

  it("shows 'Detected: nvenc' text for NVENC host", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByText(/Detected:.*nvenc/)).toBeInTheDocument();
  });

  it("shows 'No GPU accelerator detected' in backend section when detected list is empty", () => {
    mockCalibrationData = { ...HW_NVENC_DATA, detected_backends: [], hw_mode: "auto" };
    renderCard();
    expect(screen.getByText(/No GPU accelerator detected/)).toBeInTheDocument();
  });

  it("clicking HW toggle when enabled calls setMode with 'off'", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    fireEvent.click(screen.getByTestId("hw-enable-toggle"));
    expect(mockSetModeMutate).toHaveBeenCalledWith("off");
  });

  it("changing backend select calls setMode mutation with selected value", () => {
    mockCalibrationData = { ...HW_NVENC_DATA, detected_backends: ["nvenc", "vaapi"], hw_mode: "auto" };
    renderCard();
    const select = screen.getByTestId("hw-mode-select");
    fireEvent.change(select, { target: { value: "nvenc" } });
    expect(mockSetModeMutate).toHaveBeenCalledWith("nvenc");
  });

  // ── iGPU (VAAPI, gpu_class=integrated) ───────────────────────────────────────

  it("renders 'iGPU' label for gpu_class=integrated", () => {
    mockCalibrationData = HW_VAAPI_DATA;
    renderCard();
    expect(screen.getByText(/iGPU/)).toBeInTheDocument();
  });

  // ── Codec support block ───────────────────────────────────────────────────────

  it("renders codec support block when hw_codecs is non-empty", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.getByText(/Codec support/i)).toBeInTheDocument();
  });

  it("renders hw_codecs in UPPERCASE in codec support block", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    // h264, hevc, av1 → H264, HEVC, AV1
    expect(screen.getByText(/H264.*HEVC.*AV1/)).toBeInTheDocument();
  });

  it("shows software-decode-only note when a codec is encode-only", () => {
    mockCalibrationData = HW_NVENC_DATA;
    // hw_codecs has av1 but hw_decode_codecs does not
    renderCard();
    expect(screen.getByText(/AV1.*decoded in software/)).toBeInTheDocument();
  });

  it("does not render codec support block when hw_codecs and hw_decode_codecs are empty", () => {
    renderCard(); // SW_ONLY_DATA has empty codec arrays
    expect(screen.queryByText(/Codec support/i)).toBeNull();
  });

  // ── WSL banner ────────────────────────────────────────────────────────────────

  it("shows WSL VAAPI/QSV warning banner on WSL host", () => {
    mockCalibrationData = WSL_DATA;
    renderCard();
    expect(screen.getByText(/Mesa.*d3d12/i)).toBeInTheDocument();
  });

  it("does not show WSL banner on non-WSL host", () => {
    mockCalibrationData = HW_NVENC_DATA;
    renderCard();
    expect(screen.queryByText(/Mesa.*d3d12/i)).toBeNull();
  });

  it("WSL-blocked backends (vaapi, qsv) are rendered as disabled options in the select", () => {
    mockCalibrationData = WSL_DATA;
    renderCard();
    const select = screen.getByTestId("hw-mode-select");
    const vaapiOption = Array.from(select.querySelectorAll("option")).find(
      (o) => o.value === "vaapi",
    );
    expect(vaapiOption).toBeDefined();
    expect(vaapiOption?.disabled).toBe(true);
  });

  // ── HW fallbacks warning block ────────────────────────────────────────────────

  it("renders fallbacks block when hw_fallbacks is non-empty", () => {
    mockCalibrationData = HW_FALLBACKS_DATA;
    renderCard();
    expect(screen.getByText(/Hardware fallbacks/i)).toBeInTheDocument();
  });

  it("does not render fallbacks block when hw_fallbacks is empty", () => {
    renderCard(); // SW_ONLY_DATA has empty hw_fallbacks
    expect(screen.queryByText(/Hardware fallbacks/i)).toBeNull();
  });

  it("renders fallback reason label for hw_unsupported_option class", () => {
    mockCalibrationData = HW_FALLBACKS_DATA;
    renderCard();
    expect(screen.getByText(/unsupported encoder option/)).toBeInTheDocument();
  });

  it("renders fallback reason label for hw_init class", () => {
    mockCalibrationData = HW_FALLBACKS_DATA;
    renderCard();
    expect(screen.getByText(/GPU init failed/)).toBeInTheDocument();
  });

  it("renders fallback codec name in the list", () => {
    mockCalibrationData = HW_FALLBACKS_DATA;
    renderCard();
    expect(screen.getByText("hevc")).toBeInTheDocument();
  });

  // ── Recalibrate button ────────────────────────────────────────────────────────

  it("renders the Recalibrate button", () => {
    renderCard();
    expect(screen.getByTestId("recalibrate-btn")).toBeInTheDocument();
  });

  it("Recalibrate button shows 'Recalibrate' label when idle", () => {
    renderCard();
    expect(screen.getByTestId("recalibrate-btn")).toHaveTextContent(/Recalibrate$/);
  });

  it("clicking Recalibrate calls the recalibrate mutation", () => {
    renderCard();
    fireEvent.click(screen.getByTestId("recalibrate-btn"));
    expect(mockRecalibrateMutate).toHaveBeenCalledOnce();
  });

  it("Recalibrate button shows 'Recalibrating…' text when in-flight", () => {
    mockRecalibrateIsPending = true;
    renderCard();
    expect(screen.getByTestId("recalibrate-btn")).toHaveTextContent(/Recalibrating/);
  });

  it("Recalibrate button is disabled when isPending", () => {
    mockRecalibrateIsPending = true;
    renderCard();
    expect(screen.getByTestId("recalibrate-btn")).toBeDisabled();
  });

  // The POST body is the bare capability: written into the cache it blanked
  // the hw_fallbacks, hw_mode and is_wsl the GET wraps it with.
  it("recalibrate onSuccess re-reads the calibration instead of caching the POST body", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    const freshData = { ...HW_NVENC_DATA, software_max_height: 1440 };
    recalibrateOnSuccess?.(freshData);
    await waitFor(() => {
      expect(mockInvalidateQueries).toHaveBeenCalledWith({ queryKey: ["admin", "transcode-calibration"] });
      expect(toast.success).toHaveBeenCalled();
    });
    expect(mockSetQueryData).not.toHaveBeenCalledWith(["admin", "transcode-calibration"], expect.anything());
  });

  it("recalibrate onError shows 'Recalibration already in progress' toast for 409", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    recalibrateOnError?.(new Error("409 Conflict already in progress"));
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringMatching(/already in progress/i),
      );
    });
  });

  it("recalibrate onError shows generic error toast for non-409 errors", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    recalibrateOnError?.(new Error("500 Internal Server Error"));
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith(
        expect.stringMatching(/Recalibration failed/i),
      );
    });
  });

  // ── setMode mutation side-effects ─────────────────────────────────────────────

  it("setMode onSuccess shows 'Mode applied' toast", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    // The success callback writes the new mode into state: call it inside act.
    act(() => setModeOnSuccess?.({}, "auto"));
    await waitFor(() => {
      expect(toast.success).toHaveBeenCalledWith(
        expect.stringMatching(/Mode applied/i),
      );
    });
  });

  it("setMode onSuccess invalidates calibration query", async () => {
    renderCard();
    act(() => setModeOnSuccess?.({}, "nvenc"));
    await waitFor(() => {
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["admin", "transcode-calibration"] }),
      );
    });
  });

  it("setMode onError shows error toast", async () => {
    const { toast } = await import("@evey/store");
    renderCard();
    setModeOnError?.(new Error("network"));
    await waitFor(() => {
      expect(toast.error).toHaveBeenCalledWith("network");
    });
  });

  it("HW toggle is disabled while setMode mutation is pending", () => {
    mockSetModeIsPending = true;
    renderCard();
    expect(screen.getByTestId("hw-enable-toggle")).toBeDisabled();
  });

  it("backend select is disabled while setMode mutation is pending", () => {
    mockCalibrationData = HW_NVENC_DATA;
    mockSetModeIsPending = true;
    renderCard();
    expect(screen.getByTestId("hw-mode-select")).toBeDisabled();
  });
});

describe("EncodeCalibrationCard - provisional (contended) benchmark", () => {
  beforeEach(() => {
    mockCalibrationIsLoading = false;
  });

  it("flags a bench run on a busy host as provisional, with the load it saw", () => {
    mockCalibrationData = { ...SW_ONLY_DATA, bench_contended: true, bench_host_busy_percent: 87 };
    renderCard();
    const line = screen.getByTestId("calibration-contended").textContent ?? "";
    expect(line).toMatch(/provisional/i);
    expect(line).toContain("87");
    expect(line).toMatch(/measure again/i);
  });

  it("says nothing when the bench ran on an idle host", () => {
    mockCalibrationData = { ...SW_ONLY_DATA };
    renderCard();
    expect(screen.queryByTestId("calibration-contended")).toBeNull();
  });
});

describe("EncodeCalibrationCard - deferred (opportune) benchmark", () => {
  beforeEach(() => {
    mockCalibrationIsLoading = false;
  });

  it("says the ceilings are interim while the bench waits for a quiet moment, with the reason", () => {
    mockCalibrationData = { ...SW_ONLY_DATA, calibration: { state: "pending", reason: "first boot" } };
    renderCard();
    const line = screen.getByTestId("calibration-pending").textContent ?? "";
    expect(line).toMatch(/quiet moment/i);
    expect(line).toMatch(/first start/i);
    expect(line).toMatch(/interim/i);
    expect(screen.queryByTestId("calibration-benchmarking")).toBeNull();
  });

  it("says a bench is running now and that playbacks wait", () => {
    mockCalibrationData = { ...SW_ONLY_DATA, calibration: { state: "benchmarking" } };
    renderCard();
    const line = screen.getByTestId("calibration-benchmarking").textContent ?? "";
    expect(line).toMatch(/2 minutes/i);
    expect(screen.queryByTestId("calibration-pending")).toBeNull();
  });

  it("says nothing once calibrated", () => {
    mockCalibrationData = { ...SW_ONLY_DATA, calibration: { state: "calibrated" } };
    renderCard();
    expect(screen.queryByTestId("calibration-pending")).toBeNull();
    expect(screen.queryByTestId("calibration-benchmarking")).toBeNull();
  });
});

describe("EncodeCalibrationCard - remote bitrate ceiling", () => {
  beforeEach(() => {
    mockCalibrationData = HW_NVENC_DATA; // the bitrate section renders when HW is enabled
    mockSettingsData = undefined;
  });

  it("shows Follow the LAN (value 0), never 20 Mb/s, when the setting is unset", () => {
    mockSettingsData = undefined; // server never persisted the key
    renderCard();
    const sel = screen.getByTestId("max-video-bitrate-remote-select") as HTMLSelectElement;
    expect(sel.value).toBe("0");
    const opt0 = Array.from(sel.options).find((o) => o.value === "0");
    expect(opt0?.textContent ?? "").toMatch(/Follow the LAN/i);
    expect(opt0?.textContent ?? "").not.toMatch(/0\s*Mb/i);
  });

  it("keeps an explicitly persisted 20 Mb/s selected", () => {
    mockSettingsData = { maxVideoBitrateRemoteKbps: { value: 20000 } };
    renderCard();
    const sel = screen.getByTestId("max-video-bitrate-remote-select") as HTMLSelectElement;
    expect(sel.value).toBe("20000");
  });
});

// Settings and diagnostics are two cards: what the admin chooses (hardware,
// ceilings, AV1) apart from what the host measured (caps, codec support,
// recalibrate).
describe("EncodeSettingsCard / EncodeCalibrationCard split", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockCalibrationIsLoading = false;
    mockRecalibrateIsPending = false;
    mockSetModeIsPending = false;
    mockCalibrationData = HW_NVENC_DATA;
    mockSettingsData = { av1Enabled: { value: true }, av1MaxHeight: { value: 1080 } };
  });

  it("the settings card holds the choices and no measurement", () => {
    render(<EncodeSettingsCard />, { wrapper: createWrapper() });
    expect(screen.getByTestId("hw-enable-toggle")).toBeInTheDocument();
    expect(screen.getByTestId("max-video-bitrate-select")).toBeInTheDocument();
    expect(screen.queryByTestId("recalibrate-btn")).toBeNull();
    expect(screen.queryByText(/Software encode cap/i)).toBeNull();
  });

  it("the diagnostics card holds the measurements and no choice", () => {
    render(<EncodeCalibrationCard />, { wrapper: createWrapper() });
    expect(screen.getByTestId("recalibrate-btn")).toBeInTheDocument();
    expect(screen.getByText(/Software encode cap/i)).toBeInTheDocument();
    expect(screen.queryByTestId("hw-enable-toggle")).toBeNull();
    expect(screen.queryByTestId("max-video-bitrate-select")).toBeNull();
  });

  it("keeps the bitrate ceilings reachable with hardware acceleration off: they cap software encodes too", () => {
    mockCalibrationData = SW_ONLY_DATA;
    render(<EncodeSettingsCard />, { wrapper: createWrapper() });
    expect(screen.getByTestId("max-video-bitrate-select")).toBeInTheDocument();
    expect(screen.getByTestId("max-video-bitrate-remote-select")).toBeInTheDocument();
    expect(screen.queryByTestId("av1-enable-toggle")).toBeNull();
  });

  it("offers the AV1 max quality as equal options and sends the pick", () => {
    render(<EncodeSettingsCard />, { wrapper: createWrapper() });
    const group = screen.getByRole("radiogroup", { name: "AV1 max quality" });
    expect(Array.from(group.querySelectorAll("[role=radio]")).map((r) => r.getAttribute("data-value"))).toEqual(["0", "720", "1080", "1440", "2160"]);
    expect(screen.getByRole("radio", { name: "1080p" })).toHaveAttribute("aria-checked", "true");
    fireEvent.click(screen.getByRole("radio", { name: "1440p" }));
    expect(mockSetModeMutate).toHaveBeenCalledWith({ av1MaxHeight: 1440 });
  });

  it("explains every encoding choice behind an 'i'", () => {
    render(<EncodeSettingsCard />, { wrapper: createWrapper() });
    for (const label of ["Hardware acceleration", "Backend", "Video bitrate ceiling", "Video bitrate ceiling, remote viewers", "AV1 output", "AV1 max quality"]) {
      expect(screen.getByRole("button", { name: `More info: ${label}` })).toBeInTheDocument();
    }
  });
});
