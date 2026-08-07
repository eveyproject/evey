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
  mediaTitle: "Some Series S01E01",
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
  resolution: "1920x1080",
  bandwidthBps: 5000000,
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
    expect(screen.getByText("Some Series S01E01")).toBeInTheDocument();
  });

  it("shows Direct Play badge for direct_play method", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Direct Play")).toBeInTheDocument();
    expect(screen.getByTestId("play-method-badge")).toBeInTheDocument();
  });

  it("renders no play-method badge when the session has no play method", async () => {
    // Cast-resurrected and music sessions carry no play_method - rendering
    // the badge anyway produced an empty green pill.
    mockUseQuery.mockReturnValue({ data: [{ ...mockSession, playMethod: "" }], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.queryByTestId("play-method-badge")).toBeNull();
  });

  it("renders per-track decision badges with codecs and engine (copy rung)", async () => {
    // Hybrid copy rung: video copied (mkvgo), audio transcoded (ffmpeg).
    const copyRung = {
      ...mockSession,
      playMethod: "transcode",
      videoDecision: "copy",
      audioDecision: "transcode",
      videoCodec: "h264",
      audioCodec: "ac3→aac",
      resolution: "1040p",
    };
    mockUseQuery.mockReturnValue({ data: [copyRung], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    // Codec detail strings (language-neutral) prove the per-track rows rendered.
    expect(screen.getByText("(h264, 1040p)")).toBeInTheDocument();
    expect(screen.getByText("(ac3→aac)")).toBeInTheDocument();
    // Engine derived per track: copy -> mkvgo, transcode -> ffmpeg.
    expect(screen.getByText("· mkvgo")).toBeInTheDocument();
    expect(screen.getByText("· ffmpeg")).toBeInTheDocument();
  });

  it("renders no per-track badges when decisions are absent (direct play)", async () => {
    mockUseQuery.mockReturnValue({ data: [mockSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.queryByText("· mkvgo")).not.toBeInTheDocument();
    expect(screen.queryByText("· ffmpeg")).not.toBeInTheDocument();
  });

  it("shows Transcode badge for transcode method", async () => {
    const transcodeSession = { ...mockSession, playMethod: "transcode" };
    mockUseQuery.mockReturnValue({ data: [transcodeSession], isLoading: false, isError: false });
    const SessionsPage = await getSessionsPage();
    render(React.createElement(SessionsPage), { wrapper: createWrapper() });
    expect(screen.getByText("Transcode")).toBeInTheDocument();
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
