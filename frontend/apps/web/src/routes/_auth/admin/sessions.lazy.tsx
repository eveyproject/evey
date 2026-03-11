import { useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api/src/fetcher";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/sessions")({
  component: SessionsPage,
});

interface ActiveSession {
  sessionId: string;
  profileName: string;
  mediaTitle: string;
  mediaType: string;
  mediaItemId: string;
  progress: number;
  durationMs: number;
  positionMs: number;
  startedAt: string;
  deviceName: string;
  deviceType: string;
  clientIp: string;
  playMethod: string;
  videoCodec: string;
  audioCodec: string;
  resolution: string;
  bandwidthBps: number;
  transcodeInfo?: string;
}

function useActiveSessions() {
  return useQuery<ActiveSession[]>({
    queryKey: ["admin", "sessions", "active"],
    queryFn: async () => {
      const res = await eveyFetch<{ sessions: ActiveSession[] }>({
        url: "/api/v1/admin/sessions/active",
        method: "GET",
      });
      return res.sessions ?? [];
    },
    refetchInterval: 10_000,
  });
}

function useKillSession() {
  const queryClient = useQueryClient();
  return useMutation({
    mutationFn: async (sessionId: string) => {
      await eveyFetch<void>({
        url: `/api/v1/admin/sessions/${sessionId}`,
        method: "DELETE",
      });
    },
    onSuccess: () => {
      queryClient.invalidateQueries({ queryKey: ["admin", "sessions", "active"] });
      toast.success("Session stopped");
    },
    onError: (err: Error) => toast.error(err.message || "Failed to stop session"),
  });
}

function formatMs(ms: number): string {
  if (!ms) return "0:00";
  const totalSec = Math.floor(ms / 1000);
  const h = Math.floor(totalSec / 3600);
  const m = Math.floor((totalSec % 3600) / 60);
  const s = totalSec % 60;
  if (h > 0) return `${h}:${String(m).padStart(2, "0")}:${String(s).padStart(2, "0")}`;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function formatElapsed(startedAt: string): string {
  const diff = Date.now() - new Date(startedAt).getTime();
  return formatMs(diff);
}

function formatBandwidth(bps: number): string {
  if (bps > 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps > 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

function playMethodLabel(method: string): string {
  switch (method) {
    case "direct_play": return "Direct Play";
    case "direct_stream": return "Direct Stream";
    case "transcode": return "Transcode";
    default: return method;
  }
}

function DeviceIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <rect x={2} y={3} width={20} height={14} rx={2} />
      <path strokeLinecap="round" d="M8 21h8M12 17v4" />
    </svg>
  );
}

function PlayIcon() {
  return (
    <svg className="w-5 h-5" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M5.25 5.653c0-.856.917-1.398 1.667-.986l11.54 6.348a1.125 1.125 0 010 1.971l-11.54 6.347a1.125 1.125 0 01-1.667-.985V5.653z" />
    </svg>
  );
}

function StopIcon() {
  return (
    <svg className="w-4 h-4" fill="none" stroke="currentColor" strokeWidth={2} viewBox="0 0 24 24">
      <rect x={6} y={6} width={12} height={12} rx={1} />
    </svg>
  );
}

function SessionCard({ session }: { session: ActiveSession }) {
  const killSession = useKillSession();
  const [confirming, setConfirming] = useState(false);

  const progress = Math.min(100, Math.max(0, session.progress));
  const remaining = session.durationMs > 0
    ? formatMs(Math.max(0, session.durationMs - session.positionMs))
    : null;

  return (
    <div
      className="rounded-[var(--radius-lg)] overflow-hidden"
      style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
    >
      <div className="p-4 flex flex-col gap-3">
        {/* Header: user + media */}
        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <div
              className="flex items-center justify-center w-10 h-10 rounded-full shrink-0"
              style={{ backgroundColor: "var(--accent)", color: "white" }}
            >
              <span className="text-sm font-bold">
                {session.profileName.charAt(0).toUpperCase()}
              </span>
            </div>
            <div className="min-w-0">
              <p className="text-sm font-semibold truncate" style={{ color: "var(--text)" }}>
                {session.profileName}
              </p>
              <p className="text-xs truncate" style={{ color: "var(--text-muted)" }}>
                {session.clientIp}
              </p>
            </div>
          </div>
          {!confirming ? (
            <Button
              variant="ghost"
              className="shrink-0 !px-2 !py-1"
              onClick={() => setConfirming(true)}
              title="Stop session"
            >
              <StopIcon />
            </Button>
          ) : (
            <div className="flex gap-1 shrink-0">
              <Button
                variant="danger"
                className="!px-2 !py-1 text-xs"
                loading={killSession.isPending}
                onClick={() => {
                  killSession.mutate(session.sessionId);
                  setConfirming(false);
                }}
              >
                Stop
              </Button>
              <Button
                variant="ghost"
                className="!px-2 !py-1 text-xs"
                onClick={() => setConfirming(false)}
              >
                Cancel
              </Button>
            </div>
          )}
        </div>

        {/* Media info */}
        <div className="flex items-center gap-2">
          <PlayIcon />
          <div className="min-w-0 flex-1">
            <p className="text-sm font-medium truncate" style={{ color: "var(--text)" }}>
              {session.mediaTitle}
            </p>
            <div className="flex items-center gap-2 flex-wrap">
              <Badge variant={session.playMethod === "transcode" ? "warning" : "success"}>
                {playMethodLabel(session.playMethod)}
              </Badge>
              {session.resolution && (
                <span className="text-xs" style={{ color: "var(--text-muted)" }}>
                  {session.resolution}
                </span>
              )}
              {session.transcodeInfo && (
                <span className="text-xs" style={{ color: "var(--text-muted)" }}>
                  {session.transcodeInfo}
                </span>
              )}
            </div>
          </div>
        </div>

        {/* Progress bar */}
        <div>
          <div className="flex justify-between text-xs mb-1" style={{ color: "var(--text-muted)" }}>
            <span>{formatMs(session.positionMs)}</span>
            <span>
              {remaining ? `-${remaining}` : `${formatElapsed(session.startedAt)} elapsed`}
            </span>
          </div>
          <div className="h-1.5 rounded-full" style={{ backgroundColor: "var(--border)" }}>
            <div
              className="h-1.5 rounded-full transition-all duration-500"
              style={{
                width: `${progress}%`,
                backgroundColor: "var(--accent)",
              }}
            />
          </div>
        </div>

        {/* Device + codec info */}
        <div className="flex items-center gap-4 flex-wrap text-xs" style={{ color: "var(--text-muted)" }}>
          {session.deviceName && (
            <span className="flex items-center gap-1">
              <DeviceIcon />
              {session.deviceName}
              {session.deviceType && ` (${session.deviceType})`}
            </span>
          )}
          {session.videoCodec && (
            <span>Video: {session.videoCodec}</span>
          )}
          {session.audioCodec && (
            <span>Audio: {session.audioCodec}</span>
          )}
          {session.bandwidthBps > 0 && (
            <span>{formatBandwidth(session.bandwidthBps)}</span>
          )}
        </div>
      </div>
    </div>
  );
}

function SessionsPage() {
  const { data: sessions, isLoading } = useActiveSessions();

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        Now Playing
      </h1>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : !sessions || sessions.length === 0 ? (
        <EmptyState
          icon="media"
          title="No active sessions"
          description="When users start streaming, their sessions will appear here."
        />
      ) : (
        <>
          <p className="text-sm" style={{ color: "var(--text-muted)" }}>
            {sessions.length} active {sessions.length === 1 ? "session" : "sessions"} — refreshes every 10s
          </p>
          <div className="grid grid-cols-1 md:grid-cols-2 xl:grid-cols-3 gap-4">
            {sessions.map((session) => (
              <SessionCard key={session.sessionId} session={session} />
            ))}
          </div>
        </>
      )}
    </div>
  );
}
