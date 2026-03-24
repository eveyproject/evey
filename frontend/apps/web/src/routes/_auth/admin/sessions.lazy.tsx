import { useState } from "react";
import { Film, Monitor, Play, Square } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
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
    refetchIntervalInBackground: false,
  });
}

function useKillSession() {
  const { t } = useTranslation();
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
      toast.success(t("admin.sessions.sessionStopped"));
    },
    onError: (err: Error) => toast.error(err.message || t("admin.sessions.failedStop")),
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

function usePlayMethodLabel() {
  const { t } = useTranslation();
  return (method: string): string => {
    switch (method) {
      case "direct_play": return t("admin.sessions.directPlay");
      case "direct_stream": return t("admin.sessions.directStream");
      case "transcode": return t("admin.sessions.transcode");
      default: return method;
    }
  };
}

const DeviceIcon = () => <Monitor size={16} strokeWidth={1.5} />;
const PlayIcon = () => <Play size={20} strokeWidth={2} />;
const StopIcon = () => <Square size={16} strokeWidth={2} />;

function SessionCard({ session }: { session: ActiveSession }) {
  const { t } = useTranslation();
  const killSession = useKillSession();
  const playMethodLabel = usePlayMethodLabel();
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
              title={t("admin.sessions.stopSession")}
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
                {t("admin.sessions.stop")}
              </Button>
              <Button
                variant="ghost"
                className="!px-2 !py-1 text-xs"
                onClick={() => setConfirming(false)}
              >
                {t("common.cancel")}
              </Button>
            </div>
          )}
        </div>

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

        <div>
          <div className="flex justify-between text-xs mb-1" style={{ color: "var(--text-muted)" }}>
            <span>{formatMs(session.positionMs)}</span>
            <span>
              {remaining ? `-${remaining}` : `${formatElapsed(session.startedAt)} ${t("admin.sessions.elapsed")}`}
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

        <div className="flex items-center gap-4 flex-wrap text-xs" style={{ color: "var(--text-muted)" }}>
          {session.deviceName && (
            <span className="flex items-center gap-1">
              <DeviceIcon />
              {session.deviceName}
              {session.deviceType && ` (${session.deviceType})`}
            </span>
          )}
          {session.videoCodec && (
            <span>{t("admin.sessions.video", { codec: session.videoCodec })}</span>
          )}
          {session.audioCodec && (
            <span>{t("admin.sessions.audio", { codec: session.audioCodec })}</span>
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
  const { t } = useTranslation();
  const { data: sessions, isLoading, isError } = useActiveSessions();

  return (
    <div className="flex flex-col gap-6">
      <PageHeader title={t("admin.sessions.title")} />

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : isError ? (
        <EmptyState
          icon={<Film size={48} strokeWidth={1.5} />}
          title={t("admin.sessions.failedLoad")}
          description={t("admin.sessions.failedLoadDesc")}
        />
      ) : !sessions || sessions.length === 0 ? (
        <EmptyState
          icon={<Film size={48} strokeWidth={1.5} />}
          title={t("admin.sessions.noSessions")}
          description={t("admin.sessions.noSessionsDesc")}
        />
      ) : (
        <>
          <p className="text-sm" style={{ color: "var(--text-muted)" }}>
            {t("admin.sessions.activeCount", { count: sessions.length, word: sessions.length === 1 ? t("admin.sessions.session") : t("admin.sessions.sessions") })}
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
