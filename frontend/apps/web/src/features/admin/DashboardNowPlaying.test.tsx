import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { NowPlaying } from "./DashboardNowPlaying";

// ── Mutable mock state ────────────────────────────────────────────────────────

const mockKillMutate = vi.fn();
const mockSendCommandMutate = vi.fn();
let sendCommandOnError: ((err: Error) => void) | undefined;
const mockInvalidateQueries = vi.fn().mockResolvedValue(undefined);

let mockSessions: unknown[] = [];
let mockIsLoading = false;

// ── Module mocks ──────────────────────────────────────────────────────────────

vi.mock("@tanstack/react-router", () => ({
  Link: ({
    children,
    to,
    ...rest
  }: {
    children: React.ReactNode;
    to: string;
    [k: string]: unknown;
  }) => <a href={to} {...rest}>{children}</a>,
}));

vi.mock("@tanstack/react-query", async (importActual) => {
  const actual = await importActual<typeof import("@tanstack/react-query")>();
  return {
    ...actual,
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  useAdminSessionServiceListActiveSessions: () => ({
    data: { sessions: mockSessions },
    isLoading: mockIsLoading,
  }),
  useAdminSessionServiceKillSession: (opts: {
    mutation?: {
      onSuccess?: () => void;
      onError?: (err: Error) => void;
    };
  }) => ({
    mutate: (args: unknown) => {
      mockKillMutate(args);
      opts?.mutation?.onSuccess?.();
    },
    isPending: false,
  }),
  useAdminSessionServiceSendSessionCommand: (opts?: {
    mutation?: { onError?: (err: Error) => void };
  }) => {
    sendCommandOnError = opts?.mutation?.onError;
    return { mutate: mockSendCommandMutate, isPending: false };
  },
  getAdminSessionServiceListActiveSessionsQueryKey: () => ["/api/v1/admin/sessions"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (fn: (s: { accessToken: string }) => unknown) =>
    fn({ accessToken: "test-token" }),
}));

// AdminTrickplayThumbnail fetches images via the API - stub it to avoid img errors
vi.mock("./AdminTrickplayThumbnail", () => ({
  AdminTrickplayThumbnail: () => null,
}));


// ── Helpers ───────────────────────────────────────────────────────────────────

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) => (
    <QueryClientProvider client={qc}>{children}</QueryClientProvider>
  );
}

function renderNowPlaying() {
  return render(<NowPlaying />, { wrapper: createWrapper() });
}

// A minimal session fixture - string fields mirror proto string scalars
const SESSION_MOVIE = {
  sessionId: "sess-1",
  mediaItemId: "mi-movie-1",
  mediaTitle: "Nova",
  seriesTitle: undefined,
  seasonNumber: undefined,
  episodeNumber: undefined,
  profileName: "Alice",
  deviceName: "Chrome",
  playMethod: "direct_play",
  resolution: "1080p",
  progress: 42,
  positionMs: "3600000",    // 1h
  durationMs: "7200000",    // 2h
  isPaused: false,
  bandwidthBps: "5000000",  // 5 Mbps
  mediaType: "MEDIA_ITEM_TYPE_MOVIE",
};

const SESSION_EPISODE = {
  sessionId: "sess-2",
  mediaItemId: "mi-ep-1",
  mediaTitle: "Pilot",
  seriesTitle: "Some Series",
  seasonNumber: 1,
  episodeNumber: 1,
  profileName: "Bob",
  deviceName: "Firefox",
  playMethod: "transcode",
  resolution: "720p",
  progress: 10,
  positionMs: "600000",    // 10m
  durationMs: "3600000",   // 1h
  isPaused: true,
  bandwidthBps: "0",
  mediaType: "MEDIA_ITEM_TYPE_EPISODE",
};

// ── Tests ─────────────────────────────────────────────────────────────────────

describe("NowPlaying", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockSessions = [];
    mockIsLoading = false;
    sendCommandOnError = undefined;
  });

  // ── Empty state ──────────────────────────────────────────────────────────────

  it("renders the empty state when no sessions are active", () => {
    mockSessions = [];
    renderNowPlaying();
    // EmptyState renders its title prop - i18n resolves to "No active playback"
    expect(screen.getByText("No active playback")).toBeInTheDocument();
  });

  it("does not render any session card in empty state", () => {
    mockSessions = [];
    renderNowPlaying();
    expect(screen.queryByText("Nova")).toBeNull();
  });

  it("renders a spinner while loading", () => {
    mockIsLoading = true;
    mockSessions = [];
    renderNowPlaying();
    // Spinner is aria-role="status" by our Spinner component or just a div - check no card content
    expect(screen.queryByText("Nova")).toBeNull();
    expect(screen.queryByText("admin.dashboardStats.noActivePlayback")).toBeNull();
  });

  // ── Session cards rendering ──────────────────────────────────────────────────

  it("renders the media title for a movie session", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.getByText("Nova")).toBeInTheDocument();
  });

  it("renders profile name and device name for a session", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.getByText("Alice - Chrome")).toBeInTheDocument();
  });

  it("renders the session count badge when sessions are active", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    // The count badge renders the number
    expect(screen.getByText("1")).toBeInTheDocument();
  });

  it("renders both session cards when two sessions are active", () => {
    mockSessions = [SESSION_MOVIE, SESSION_EPISODE];
    renderNowPlaying();
    expect(screen.getByText("Nova")).toBeInTheDocument();
    expect(screen.getByText("Some Series · S01E01 · Pilot")).toBeInTheDocument();
  });

  it("renders series title for an episode session", () => {
    mockSessions = [SESSION_EPISODE];
    renderNowPlaying();
    expect(screen.getByText("Some Series · S01E01 · Pilot")).toBeInTheDocument();
  });

  it("renders S01E01 episode label for a series session with season+episode numbers", () => {
    mockSessions = [SESSION_EPISODE];
    renderNowPlaying();
    expect(screen.getByText(/S01E01/)).toBeInTheDocument();
  });

  it("renders episode-only label (no season) when seasonNumber is 0", () => {
    const epNoSeason = { ...SESSION_EPISODE, seasonNumber: 0, episodeNumber: 3 };
    mockSessions = [epNoSeason];
    renderNowPlaying();
    expect(screen.getByText("Some Series · E03 · Pilot")).toBeInTheDocument();
  });

  it("does not render episode label for a movie session", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.queryByText(/^S\d{2}E\d{2}$/)).toBeNull();
  });

  it("renders the resolution for a session that has one", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.getByText("1080p")).toBeInTheDocument();
  });

  it("renders the play-method badge for a session that declared one", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.getByTestId("play-method-badge")).toBeInTheDocument();
  });

  it("renders no play-method badge for a session without a play method", () => {
    // Cast-resurrected and music sessions carry no play_method - rendering
    // the badge anyway produced an empty green pill.
    mockSessions = [{ ...SESSION_MOVIE, playMethod: "" }];
    renderNowPlaying();
    expect(screen.queryByTestId("play-method-badge")).toBeNull();
  });

  // The dashboard card used to say only "Transcode": it now names what serves
  // each track, from what the server recorded when it rendered the master.
  it("renders the per-track serving lines (engine + backend, player-side decode)", () => {
    mockSessions = [{
      ...SESSION_MOVIE,
      playMethod: "transcode",
      videoDecision: "transcode", videoEngine: "ffmpeg", videoBackend: "qsv", videoCodec: "hevc→h264",
      audioDecision: "wasm", audioEngine: "client", audioCodec: "eac3",
    }];
    renderNowPlaying();
    expect(screen.getByTestId("session-track-badges")).toBeInTheDocument();
    expect(screen.getByText("· ffmpeg Quick Sync")).toBeInTheDocument();
    expect(screen.getByText("· ac3go in the player")).toBeInTheDocument();
    expect(screen.getByText("(hevc→h264, 1080p)")).toBeInTheDocument();
  });

  it("renders no serving lines for a direct play (no per-track decision)", () => {
    mockSessions = [{ ...SESSION_MOVIE, videoDecision: "", audioDecision: "" }];
    renderNowPlaying();
    expect(screen.queryByTestId("session-track-badges")).toBeNull();
  });

  // ── Progress / time formatting ────────────────────────────────────────────────

  it("renders formatted position time (1:00:00 for 1h)", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    // positionMs = 3600000 → 1:00:00
    expect(screen.getByText("1:00:00")).toBeInTheDocument();
  });

  it("renders remaining time with leading dash (-1:00:00 for 1h left)", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    // durationMs=7200000, positionMs=3600000 → remaining=3600000 → -1:00:00
    expect(screen.getByText("-1:00:00")).toBeInTheDocument();
  });

  it("renders mm:ss position for a sub-hour session (10:00 for 10m)", () => {
    mockSessions = [SESSION_EPISODE];
    renderNowPlaying();
    // positionMs=600000 → 10:00
    expect(screen.getByText("10:00")).toBeInTheDocument();
  });

  it("renders zero position as 0:00 when positionMs is 0", () => {
    mockSessions = [{ ...SESSION_MOVIE, positionMs: "0", durationMs: "0", progress: 0 }];
    renderNowPlaying();
    // Both position and remaining will display 0:00 / "-"
    expect(screen.getAllByText("0:00").length).toBeGreaterThanOrEqual(1);
  });

  it("renders bandwidth when bandwidthBps is above zero (5 Mbps)", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.getByText("5.0 Mbps")).toBeInTheDocument();
  });

  it("does not render bandwidth line when bandwidthBps is 0", () => {
    mockSessions = [SESSION_EPISODE];
    renderNowPlaying();
    expect(screen.queryByText(/Mbps|Kbps|bps/)).toBeNull();
  });

  // ── Play/Pause button ─────────────────────────────────────────────────────────

  it("shows Pause button aria-label when session is not paused", () => {
    mockSessions = [SESSION_MOVIE]; // isPaused=false
    renderNowPlaying();
    // t("common.pause") falls back to the key when not in static translations
    expect(screen.getByRole("button", { name: /^pause$/i })).toBeInTheDocument();
  });

  it("shows Play button aria-label when session is paused", () => {
    mockSessions = [SESSION_EPISODE]; // isPaused=true
    renderNowPlaying();
    expect(screen.getByRole("button", { name: /^play$/i })).toBeInTheDocument();
  });

  it("clicking play/pause sends 'pause' command for a playing session", () => {
    mockSessions = [SESSION_MOVIE]; // isPaused=false
    renderNowPlaying();
    fireEvent.click(screen.getByRole("button", { name: /^pause$/i }));
    expect(mockSendCommandMutate).toHaveBeenCalledWith({
      sessionId: "sess-1",
      data: { command: "pause" },
    });
  });

  it("clicking play/pause sends 'play' command for a paused session", () => {
    mockSessions = [SESSION_EPISODE]; // isPaused=true
    renderNowPlaying();
    fireEvent.click(screen.getByRole("button", { name: /^play$/i }));
    expect(mockSendCommandMutate).toHaveBeenCalledWith({
      sessionId: "sess-2",
      data: { command: "play" },
    });
  });

  // ── Stop/kill session ─────────────────────────────────────────────────────────

  it("stop button is rendered with accessible label", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    // t("admin.sessions.stopSession") → "Stop session"
    expect(screen.getByRole("button", { name: /stop session/i })).toBeInTheDocument();
  });

  // Confirm-to-act: the first click only ARMS the button (it relabels to
  // "Confirm"), the second click kills. Replaced window.confirm.
  function clickStopTwice() {
    fireEvent.click(screen.getByRole("button", { name: /stop session/i }));
    fireEvent.click(screen.getByRole("button", { name: /confirm/i }));
  }

  it("two clicks call killSession mutation with the session id", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    clickStopTwice();
    expect(mockKillMutate).toHaveBeenCalledWith({ sessionId: "sess-1" });
  });

  it("a single click only arms the button - nothing is killed", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    fireEvent.click(screen.getByRole("button", { name: /stop session/i }));
    expect(mockKillMutate).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: /confirm/i })).toBeInTheDocument();
  });

  it("successful kill invalidates the active sessions query", async () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    clickStopTwice();
    await waitFor(() =>
      expect(mockInvalidateQueries).toHaveBeenCalledWith(
        expect.objectContaining({ queryKey: ["/api/v1/admin/sessions"] }),
      ),
    );
  });

  it("shows success toast after session is stopped", async () => {
    const { toast } = await import("@evey/store");
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    clickStopTwice();
    await waitFor(() => expect(toast.success).toHaveBeenCalled());
  });

  // ── Send message ──────────────────────────────────────────────────────────────

  it("message form is hidden by default", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    expect(screen.queryByRole("textbox")).toBeNull();
  });

  it("clicking the message button reveals the message input", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    const msgBtn = screen.getByRole("button", {
      name: /send message/i,
    });
    fireEvent.click(msgBtn);
    expect(screen.getByRole("textbox")).toBeInTheDocument();
  });

  it("submitting a message sends a message: command with the session id", async () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    fireEvent.click(
      screen.getByRole("button", { name: /send message/i }),
    );
    const input = screen.getByRole("textbox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "Hello viewer!" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() =>
      expect(mockSendCommandMutate).toHaveBeenCalledWith({
        sessionId: "sess-1",
        data: { command: "message:Hello viewer!" },
      }),
    );
  });

  it("empty message does not call sendCommand", async () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    fireEvent.click(
      screen.getByRole("button", { name: /send message/i }),
    );
    const input = screen.getByRole("textbox") as HTMLInputElement;
    fireEvent.change(input, { target: { value: "   " } });
    fireEvent.submit(input.closest("form")!);
    expect(mockSendCommandMutate).not.toHaveBeenCalled();
  });

  it("message form hides after successful send", async () => {
    const { toast } = await import("@evey/store");
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    fireEvent.click(
      screen.getByRole("button", { name: /send message/i }),
    );
    const input = screen.getByRole("textbox");
    fireEvent.change(input, { target: { value: "Hello!" } });
    fireEvent.submit(input.closest("form")!);
    await waitFor(() => expect(screen.queryByRole("textbox")).toBeNull());
    expect(toast.success).toHaveBeenCalled();
  });

  // ── View-all link ─────────────────────────────────────────────────────────────

  it("renders the 'View all' link to /admin/sessions", () => {
    renderNowPlaying();
    const link = screen.getByRole("link", { name: /view all/i });
    expect(link).toHaveAttribute("href", "/admin/sessions");
  });

  // ── sendCommand error handling ────────────────────────────────────────────────

  it("sendCommand registers an onError handler so command failures are not silent", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    // The hook must have been called with mutation.onError so failures surface as toasts.
    expect(sendCommandOnError).toBeTypeOf("function");
  });

  it("sendCommand onError calls toast.error with the error message", async () => {
    const { toast } = await import("@evey/store");
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    // Simulate the mutation failing after the component registered onError
    sendCommandOnError?.(new Error("stream gone"));
    expect(toast.error).toHaveBeenCalledWith("stream gone");
  });

  it("send-message input has an accessible label", () => {
    mockSessions = [SESSION_MOVIE];
    renderNowPlaying();
    fireEvent.click(
      screen.getByRole("button", { name: /send message/i }),
    );
    // aria-label mirrors the placeholder key so screen readers announce the field purpose
    expect(
      screen.getByRole("textbox", { name: /type a message/i }),
    ).toBeInTheDocument();
  });
});
