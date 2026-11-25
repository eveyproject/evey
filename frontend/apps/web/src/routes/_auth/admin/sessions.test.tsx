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
    useQuery: (...args: unknown[]) => mockUseQuery(...args),
    useMutation: (...args: unknown[]) => mockUseMutation(...args),
    useQueryClient: () => ({ invalidateQueries: mockInvalidateQueries }),
  };
});

vi.mock("@evey/api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@evey/api")>()),
  eveyFetch: vi.fn(),
  useAdminSessionServiceListActiveSessions: () => {
    const result = mockUseQuery();
    const sessions = Array.isArray(result.data) ? result.data : [];
    return { ...result, data: result.data != null ? { sessions } : undefined };
  },
  useAdminSessionServiceKillSession: () => mockUseMutation(),
  getAdminSessionServiceListActiveSessionsQueryKey: () => ["/api/v1/admin/sessions"],
}));

vi.mock("@evey/store", () => ({
  toast: { success: vi.fn(), error: vi.fn() },
  useAuthStore: (fn: (s: { accessToken: string }) => unknown) => fn({ accessToken: "test-token" }),
}));

vi.mock("@tanstack/react-router", () => ({
  createLazyFileRoute: () => (opts: { component: React.ComponentType }) => opts,
}));

function createWrapper() {
  const qc = new QueryClient({ defaultOptions: { queries: { retry: false } } });
  return ({ children }: { children: React.ReactNode }) =>
    React.createElement(QueryClientProvider, { client: qc }, children);
}

async function getSessionsPage() {
  const mod = await import("./sessions.lazy");
  return (mod.Route as unknown as { component: React.ComponentType }).component;
}

const mockSession = {
  sessionId: "sess-1",
  profileName: "Alice",
  mediaTitle: "Pilot",
  seriesTitle: "Some Series",
  seasonNumber: 1,
  episodeNumber: 1,
  mediaType: "episode",
  mediaItemId: "media-1",
  progress: 42,
  durationMs: 3600000,
  positionMs: 1512000,
  startedAt: new Date(Date.now() - 60000).toISOString(),
  deviceName: "Chrome",
  deviceType: "browser",
  clientIp: "192.168.1.10",
  playMethod: "direct_play",
  videoCodec: "h264",
  audioCodec: "aac",
  resolution: "1080p",
  bandwidthBps: 5000000,
  servedVideo: { engine: "file", copy: true, track: -1, codec: "h264", height: 1080 },
  servedAudio: { engine: "file", copy: true, track: -1 },
};

describe("SessionsPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockUseMutation.mockReturnValue({ mutate: vi.fn(), isPending: false });
  });

  it("renders page heading", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Now Playing")).toBeInTheDocument();
  });

  it("shows spinner while loading", async () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: true, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Now Playing")).toBeInTheDocument();
  });

  it("shows empty state when no sessions", async () => {
    mockUseQuery.mockReturnValue({ data: [], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText(/No active sessions/i)).toBeInTheDocument();
  });

  it("shows error state on fetch failure", async () => {
    mockUseQuery.mockReturnValue({ data: undefined, isLoading: false, isError: true });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText(/Failed to load sessions/i)).toBeInTheDocument();
  });

  it("renders session card with user and media title", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Alice")).toBeInTheDocument();
    expect(screen.getByText("Some Series · S01E01 · Pilot")).toBeInTheDocument();
  });

  it("shows Direct Play badge for direct_play method", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Direct Play")).toBeInTheDocument();
    expect(screen.getByTestId("play-method-badge")).toBeInTheDocument();
  });

  it("renders no play-method badge before any byte was served", async () => {
    // A session a start just created, a cast-resurrected or a music session:
    // nothing served names no method, not an empty green pill.
    mockUseQuery.mockReturnValue({ data: [{ ...mockSession, servedVideo: undefined, servedAudio: undefined }], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.queryByTestId("play-method-badge")).toBeNull();
  });

  it("renders the served tracks with their codecs and engines (hybrid)", async () => {
    // Video copied by the packager, MP3 track 2 encoded to AAC by ffmpeg.
    const hybrid = {
      ...mockSession,
      servedVideo: { engine: "mkvgo", copy: true, track: -1, codec: "h264", height: 1040 },
      servedAudio: { engine: "ffmpeg", track: 1, codec: "aac", sourceCodec: "mp3" },
    };
    mockUseQuery.mockReturnValue({ data: [hybrid], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("(h264, 1040p)")).toBeInTheDocument();
    expect(screen.getByText("(mp3→aac)")).toBeInTheDocument();
    expect(screen.getByText("· mkvgo")).toBeInTheDocument();
    expect(screen.getByText("· ffmpeg")).toBeInTheDocument();
    expect(screen.getByText("Transcode")).toBeInTheDocument();
  });

  it("names the encode backend and the player's wasm decoder", async () => {
    const wasm = {
      ...mockSession,
      servedVideo: { engine: "ffmpeg", codec: "h264", sourceCodec: "hevc", backend: "qsv" },
      servedAudio: { engine: "ffmpeg", copy: true, track: 0, codec: "eac3" },
      played: { source: "hls", audioTrack: 0, audioCodec: "ec-3", audioDecoder: "wasm" },
    };
    mockUseQuery.mockReturnValue({ data: [wasm], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("· ffmpeg Quick Sync")).toBeInTheDocument();
    expect(screen.getByTestId("played")).toHaveTextContent("decoded by ac3go (wasm)");
  });

  it("names a direct play by the source file, no engine", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.queryByText("· mkvgo")).not.toBeInTheDocument();
    expect(screen.queryByText("· ffmpeg")).not.toBeInTheDocument();
    expect(screen.getByTestId("served-video")).toHaveTextContent("source file");
  });

  // The case measured on the demo: the player said MP3 while the server kept
  // sending the AAC track. Both facts show, and the card says they differ.
  it("says so when the player plays another track than the one sent", async () => {
    const off = {
      ...mockSession,
      servedVideo: { engine: "mkvgo", copy: true, codec: "h264" },
      servedAudio: { engine: "mkvgo", copy: true, track: 2, codec: "aac" },
      played: { source: "hls", audioTrack: 1, audioCodec: "mp4a.40.2", audioDecoder: "native" },
    };
    mockUseQuery.mockReturnValue({ data: [off], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByTestId("mismatch-track")).toHaveTextContent("the player plays track 2, the server sends track 3");
  });

  it("clicking stop button shows confirm/cancel", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    const stopBtn = screen.getByTitle("Stop session");
    fireEvent.click(stopBtn);
    await waitFor(() => {
      expect(screen.getByRole("button", { name: /Stop/i })).toBeInTheDocument();
      expect(screen.getByRole("button", { name: /Cancel/i })).toBeInTheDocument();
    });
  });

  it("cancel hides confirm buttons", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    fireEvent.click(screen.getByTitle("Stop session"));
    await waitFor(() => expect(screen.getByRole("button", { name: /Cancel/i })).toBeInTheDocument());
    fireEvent.click(screen.getByRole("button", { name: /Cancel/i }));
    await waitFor(() => expect(screen.queryByRole("button", { name: /Cancel/i })).not.toBeInTheDocument());
  });
});
