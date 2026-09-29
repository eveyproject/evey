import { useState, useEffect } from "react";
import { Cast, Film, Monitor, Square } from "lucide-react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminSessionServiceListActiveSessions,
  useAdminSessionServiceKillSession,
  getAdminSessionServiceListActiveSessionsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import type { V1ActiveSession } from "@evey/api";
import { formatTimeMs, isRealTranscode } from "@evey/api";
import { useSessionPlayMethodLabel } from "@/features/admin/playMethodLabel";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { PageHeader } from "@/components/ui/PageHeader";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { SessionTrackBadges } from "@/features/admin/SessionTrackBadges";
import { EmptyState } from "@/components/ui/EmptyState";
import { AdminTrickplayThumbnail } from "@/features/admin/AdminTrickplayThumbnail";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/sessions")({
  component: SessionsPage,
});

function formatElapsed(startedAt: string): string {
  const diff = Date.now() - new Date(startedAt).getTime();
  return formatTimeMs(diff);
}

function formatBandwidth(bps: number): string {
  if (bps > 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps > 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

const DeviceIcon = () => <Monitor size={16} strokeWidth={1.5} />;
const StopIcon = () => <Square size={16} strokeWidth={2} />;

function ProfileAvatar({ profileName, size = 40 }: { profileName?: string; size?: number }) {
  const initial = (profileName ?? "?").charAt(0).toUpperCase();
  return (
    <div
      className="rounded-full shrink-0 bg-[var(--accent)] text-[var(--filter-active-text)] flex items-center justify-center overflow-hidden"
      style={{ width: size, height: size, fontSize: size * 0.4 }}
    >
      <span className="font-bold">{initial}</span>
    </div>
  );
}

function SessionCard({ session }: { session: V1ActiveSession }) {
  const { t } = useTranslation();
  const { demo, lock } = useDemoLock();
  const queryClient = useQueryClient();
  const killSession = useAdminSessionServiceKillSession({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: getAdminSessionServiceListActiveSessionsQueryKey() });
        toast.success(t("admin.sessions.sessionStopped"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.sessions.failedStop"))),
    },
  });
  const playMethodLabel = useSessionPlayMethodLabel();
  // Cast-resurrected and music sessions carry no play_method; an empty label
  // must drop the badge entirely, not render a blank green pill.
  const methodLabel = playMethodLabel(session);
  const [confirming, setConfirming] = useState(false);

  const progress = Math.min(100, Math.max(0, session.progress ?? 0));
  const durationMs = parseInt(session.durationMs ?? "0");
  const positionMs = parseInt(session.positionMs ?? "0");
  const bandwidthBps = parseInt(session.bandwidthBps ?? "0");
  const remaining = durationMs > 0
    ? formatTimeMs(Math.max(0, durationMs - positionMs))
    : null;

  return (
    <div
      className="rounded-[var(--radius-lg)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]"
    >
      <AdminTrickplayThumbnail
        mediaId={session.mediaItemId ?? ""}
        positionMs={parseInt(session.positionMs ?? "0")}
        mediaType={session.mediaType}
        fullWidth
      />
      <div className="px-4 pt-1.5 pb-1">
        <div className="h-1.5 rounded-full bg-[var(--border)]">
          <div
            className="h-1.5 rounded-full transition-all duration-500 bg-[var(--accent)]"
            style={{ width: `${progress}%` }}
          />
        </div>
        <div className="flex justify-between text-xs mt-1 text-[var(--text-muted)]">
          <span>{formatTimeMs(positionMs)}</span>
          <span>
            {remaining ? `-${remaining}` : `${formatElapsed(session.startedAt ?? "")} ${t("admin.sessions.elapsed")}`}
          </span>
        </div>
      </div>

      <div className="px-4 pb-4 flex flex-col gap-3">
        <div>
          <p className="text-sm font-semibold truncate text-[var(--text)]">
            {session.mediaTitle}
          </p>
          {methodLabel && (
            <div className="flex items-center gap-2 flex-wrap mt-1">
              <Badge data-testid="play-method-badge" variant={isRealTranscode(session) ? "warning" : "success"}>
                {methodLabel}
              </Badge>
            </div>
          )}
          <SessionTrackBadges session={session} />
        </div>

        <div className="flex items-start justify-between gap-3">
          <div className="flex items-center gap-3 min-w-0">
            <ProfileAvatar profileName={session.profileName} />
            <div className="min-w-0">
              <p className="text-sm font-semibold truncate text-[var(--text)]">
                {session.profileName}
              </p>
              {/* Two households can both name a profile "Papa"; the account is
                  what makes the card identify one person. */}
              <p className="text-xs truncate text-[var(--text-muted)]">
                {session.username ? `${session.username} - ${session.clientIp}` : session.clientIp}
              </p>
            </div>
          </div>
          {!confirming ? (
            <Button
              variant="ghost"
              className="shrink-0 !px-2 !py-1"
              onClick={() => setConfirming(true)}
              title={demo ? t("common.demo.locked") : t("admin.sessions.stopSession")}
              {...lock}
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
                  killSession.mutate({ sessionId: session.sessionId ?? "" });
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

        <div className="flex items-center gap-4 flex-wrap text-xs text-[var(--text-muted)]">
          {session.deviceName && (
            <span className="flex items-center gap-1">
              <DeviceIcon />
              {session.deviceName}
              {session.deviceType && ` (${session.deviceType})`}
            </span>
          )}
          {session.castDeviceName && (
            <span
              className="flex items-center gap-1 text-[var(--accent)]"
              title={t("admin.sessions.castingTo", { device: session.castDeviceName })}
            >
              <Cast size={12} aria-hidden="true" />
              {session.castDeviceName}
            </span>
          )}
          {session.videoCodec && (
            <span>{t("admin.sessions.video", { codec: session.videoCodec })}</span>
          )}
          {session.audioCodec && (
            <span>{t("admin.sessions.audio", { codec: session.audioCodec })}</span>
          )}
          {bandwidthBps > 0 && (
            <span>
              {t("admin.sessions.servedBitrate", {
                rate: formatBandwidth(bandwidthBps),
              })}
            </span>
          )}
        </div>
      </div>
    </div>
  );
}

const REFRESH_INTERVAL = 3_000;

function RefreshStatus({ dataUpdatedAt, intervalMs }: { dataUpdatedAt: number; intervalMs: number }) {
  const { t } = useTranslation();
  const [, setTick] = useState(0);
  useEffect(() => {
    const id = setInterval(() => setTick((n) => n + 1), 1000);
    return () => clearInterval(id);
  }, []);
  const updatedAt = dataUpdatedAt ? new Date(dataUpdatedAt).toLocaleTimeString() : "";
  return (
    <span className="text-xs text-[var(--text-muted)]">
      {t("admin.sessions.refreshStatus", { time: updatedAt, seconds: Math.round(intervalMs / 1000) })}
    </span>
  );
}

function SessionsPage() {
  const { t } = useTranslation();
  const { data, isLoading, isError, dataUpdatedAt } = useAdminSessionServiceListActiveSessions({
    query: { refetchInterval: REFRESH_INTERVAL, refetchIntervalInBackground: false },
  });
  const sessions = data?.sessions ?? [];

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
      ) : sessions.length === 0 ? (
        <EmptyState
          icon={<Film size={48} strokeWidth={1.5} />}
          title={t("admin.sessions.noSessions")}
          description={t("admin.sessions.noSessionsDesc")}
        />
      ) : (
        <>
          <div className="flex items-center justify-between">
            <p className="text-sm text-[var(--text-muted)]">
              {t("admin.sessions.activeCount", { count: sessions.length })}
            </p>
            <RefreshStatus dataUpdatedAt={dataUpdatedAt} intervalMs={REFRESH_INTERVAL} />
          </div>
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
