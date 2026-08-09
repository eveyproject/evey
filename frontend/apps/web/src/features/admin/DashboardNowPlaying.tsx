import { Avatar } from "@/components/ui/Avatar";
import { useEffect, useState } from "react";
import { Cast, Film, Music, Play, Pause, Square, MessageSquare, Send } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminSessionServiceListActiveSessions,
  useAdminSessionServiceKillSession,
  useAdminSessionServiceSendSessionCommand,
  getAdminSessionServiceListActiveSessionsQueryKey,
} from "@evey/api";
import type { V1ActiveSession } from "@evey/api";
import { formatTimeMs } from "@evey/api";
import { toast, useAuthStore } from "@evey/store";
import { Badge } from "@/components/ui/Badge";
import { AdminTrickplayThumbnail } from "./AdminTrickplayThumbnail";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";

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

function ProfileAvatar({ profileName, size = 28 }: { profileName?: string; size?: number }) {
  return <Avatar initial={(profileName ?? "?").charAt(0).toUpperCase()} size={size} />;
}

function formatEpisodeLabel(session: V1ActiveSession): string | null {
  if (!session.seriesTitle) return null;
  const s = session.seasonNumber ?? 0;
  const e = session.episodeNumber ?? 0;
  if (s > 0 && e > 0) return `S${String(s).padStart(2, "0")}E${String(e).padStart(2, "0")}`;
  if (e > 0) return `E${String(e).padStart(2, "0")}`;
  return null;
}

function formatBandwidth(bps: number): string {
  if (bps > 1e6) return `${(bps / 1e6).toFixed(1)} Mbps`;
  if (bps > 1e3) return `${(bps / 1e3).toFixed(0)} Kbps`;
  return `${bps} bps`;
}

// NowPlayingAlbumArt renders a music session's album cover (auth'd via a token
// query param, since an <img> can't send a bearer header), the square-art
// counterpart of the video trickplay thumbnail.
function NowPlayingAlbumArt({ coverArtUrl }: { coverArtUrl?: string }) {
  const token = useAuthStore((s) => s.accessToken);
  const src = coverArtUrl ? `${coverArtUrl}${token ? `?token=${encodeURIComponent(token)}` : ""}` : "";
  return (
    <div className="relative w-full aspect-video bg-[var(--overlay-scrim)] flex items-center justify-center overflow-hidden">
      {src ? (
        <>
          <div
            className="absolute inset-0 bg-cover bg-center blur-xl opacity-40 scale-110"
            style={{ backgroundImage: `url(${src})` }}
            aria-hidden="true"
          />
          <img src={src} alt="" className="relative h-full aspect-square object-cover rounded shadow-lg" />
        </>
      ) : (
        <Music size={40} strokeWidth={1.5} className="text-[var(--text-muted)]" aria-hidden="true" />
      )}
    </div>
  );
}

function NowPlayingCard({ session }: { session: V1ActiveSession }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const killSession = useAdminSessionServiceKillSession({
    mutation: {
      onSuccess: () => {
        queryClient.invalidateQueries({ queryKey: getAdminSessionServiceListActiveSessionsQueryKey() });
        toast.success(t("admin.sessions.sessionStopped"));
      },
      onError: (err: Error) => toast.error(err.message || t("admin.sessions.failedStop")),
    },
  });
  const sendCommand = useAdminSessionServiceSendSessionCommand({
    mutation: {
      onError: (err: Error) => toast.error(err.message || t("admin.sessions.failedStop")),
    },
  });
  const playMethodLabel = usePlayMethodLabel();
  // Cast-resurrected and music sessions carry no play_method; an empty label
  // must drop the badge entirely, not render a blank green pill.
  const methodLabel = playMethodLabel(session.playMethod ?? "");
  const [showMsg, setShowMsg] = useState(false);
  // Confirm-to-act (same pattern as the dashboard's Restart): one click arms,
  // a second within a few seconds kills the session. Replaces window.confirm.
  const [stopArmed, setStopArmed] = useState(false);
  useEffect(() => {
    if (!stopArmed) return;
    const id = setTimeout(() => setStopArmed(false), 4000);
    return () => clearTimeout(id);
  }, [stopArmed]);
  const [msgText, setMsgText] = useState("");
  const progress = Math.min(100, Math.max(0, session.progress ?? 0));
  const positionMs = parseInt(session.positionMs ?? "0");
  const durationMs = parseInt(session.durationMs ?? "0");
  const remaining = durationMs > 0 ? formatTimeMs(Math.max(0, durationMs - positionMs)) : null;
  const epLabel = formatEpisodeLabel(session);
  const bandwidthBps = parseInt(session.bandwidthBps ?? "0");

  return (
    <div className="rounded-[var(--radius-lg)] overflow-hidden bg-[var(--surface)] border border-[var(--border)]">
      {session.mediaType === "track" ? (
        <NowPlayingAlbumArt coverArtUrl={session.coverArtUrl} />
      ) : (
        <AdminTrickplayThumbnail
          mediaId={session.mediaItemId ?? ""}
          positionMs={positionMs}
          mediaType={session.mediaType}
          fullWidth
        />
      )}
      <div className="px-3 pt-1.5 pb-1">
        <div className="h-1.5 rounded-full bg-[var(--border)]">
          <div
            className="h-1.5 rounded-full transition-all duration-500 bg-[var(--accent)]"
            style={{ width: `${progress}%` }}
          />
        </div>
        <div className="flex justify-between text-xs mt-1 text-[var(--text-muted)]">
          <span>{formatTimeMs(positionMs)}</span>
          <span>{remaining ? `-${remaining}` : "-"}</span>
        </div>
      </div>

      <div className="px-3 flex flex-col gap-1.5 py-1.5">
        <div className="flex items-center justify-center gap-3">
          <button
            className="p-2 rounded-full hover:bg-[var(--border)] transition-colors text-[var(--text)]"
            onClick={() => {
              const cmd = session.isPaused ? "play" : "pause";
              sendCommand.mutate({ sessionId: session.sessionId ?? "", data: { command: cmd } });
            }}
            title={session.isPaused ? t("common.play") : t("common.pause")}
            aria-label={session.isPaused ? t("common.play") : t("common.pause")}
          >
            {session.isPaused ? <Play size={ICON.lg} fill="currentColor"  aria-hidden="true" /> : <Pause size={ICON.lg}  aria-hidden="true" />}
          </button>
          <button
            className={`p-2 rounded-full transition-colors ${stopArmed ? "bg-[var(--danger-bg,#dc2626)] text-[var(--on-danger,#fff)]" : "hover:bg-[var(--danger-hover,#b91c1c)] text-[var(--danger-bg,#dc2626)]"}`}
            onClick={() => {
              if (!stopArmed) {
                setStopArmed(true);
                return;
              }
              setStopArmed(false);
              killSession.mutate({ sessionId: session.sessionId ?? "" });
            }}
            title={stopArmed ? t("common.confirm") : t("admin.sessions.stopSession")}
            aria-label={stopArmed ? t("common.confirm") : t("admin.sessions.stopSession")}
          >
            <Square size={16} fill="currentColor"  aria-hidden="true" />
          </button>
          <button
            className="p-2 rounded-full hover:bg-[var(--border)] transition-colors text-[var(--text-muted)] hover:text-[var(--text)]"
            onClick={() => setShowMsg(!showMsg)}
            title={t("admin.sessions.sendMessage")}
            aria-label={t("admin.sessions.sendMessage")}
          >
            <MessageSquare size={16}  aria-hidden="true" />
          </button>
        </div>
        {showMsg && (
          <form
            className="flex items-center gap-1.5"
            onSubmit={(e) => {
              e.preventDefault();
              if (!msgText.trim()) return;
              sendCommand.mutate({
                sessionId: session.sessionId ?? "",
                data: { command: `message:${msgText.trim()}` },
              });
              setMsgText("");
              setShowMsg(false);
              toast.success(t("admin.sessions.messageSent"));
            }}
          >
            <input
              autoFocus
              aria-label={t("admin.sessions.messagePlaceholder")}
              className="flex-1 text-xs px-2 py-1 rounded bg-[var(--input-bg,var(--surface))] border border-[var(--border)] text-[var(--text)] placeholder:text-[var(--text-muted)]"
              placeholder={t("admin.sessions.messagePlaceholder")}
              value={msgText}
              onChange={(e) => setMsgText(e.target.value)}
            />
            <button
              type="submit"
              className="p-1.5 rounded-full hover:bg-[var(--border)] transition-colors text-[var(--accent)]"
              disabled={!msgText.trim()}
              aria-label={t("admin.sessions.sendMessage")}
            >
              <Send size={14}  aria-hidden="true" />
            </button>
          </form>
        )}
      </div>

      <div className="px-3 pb-3 flex flex-col gap-2">
        <div>
          {session.seriesTitle && (
            <p className="text-xs font-medium text-[var(--accent)] truncate">{session.seriesTitle}</p>
          )}
          <p className="text-sm font-semibold truncate text-[var(--text)]">
            {epLabel && <span className="text-[var(--text-muted)] mr-1">{epLabel}</span>}
            {session.mediaTitle}
          </p>
        </div>

        <div className="flex items-center gap-2">
          <ProfileAvatar profileName={session.profileName} size={24} />
          <span className="text-xs truncate text-[var(--text-muted)] flex items-center gap-1 min-w-0">
            <span className="truncate">
              {session.profileName}
              {session.deviceName ? ` - ${session.deviceName}` : ""}
            </span>
            {session.castDeviceName && (
              <span
                className="inline-flex items-center gap-0.5 shrink-0 text-[var(--accent)]"
                title={t("admin.sessions.castingTo", { device: session.castDeviceName })}
              >
                <Cast size={12} aria-hidden="true" />
                {session.castDeviceName}
              </span>
            )}
          </span>
          <div className="ml-auto flex items-center gap-2 shrink-0">
            {/* The live decisions outrank the play method the client declared
                at start: a cast begins on the server's AAC transcode, then the
                TV's wasm decoder takes over and the server ffmpeg dies - the
                receiver's audio_mode beacon rewrites audioDecision, so the
                badge follows the REAL path in near real time. */}
            {session.audioDecision === "wasm" && session.videoDecision !== "transcode" ? (
              <Badge variant="info">{t("player.streamInfo.decisionWasm")}</Badge>
            ) : methodLabel ? (
              <Badge data-testid="play-method-badge" variant={session.playMethod === "transcode" ? "warning" : "success"}>
                {methodLabel}
              </Badge>
            ) : null}
            {session.resolution && (
              <span className="text-xs text-[var(--text-muted)]">{session.resolution}</span>
            )}
          </div>
        </div>

        {bandwidthBps > 0 && (
          <div className="text-xs text-[var(--text-muted)]">
            {formatBandwidth(bandwidthBps)}
          </div>
        )}
      </div>
    </div>
  );
}

export function NowPlaying() {
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useAdminSessionServiceListActiveSessions({
    query: { refetchInterval: 3_000, refetchIntervalInBackground: false },
  });
  const sessions = data?.sessions ?? [];

  return (
    <div>
      <div className="flex items-center justify-between mb-3">
        <div className="flex items-center gap-2">
          <Play size={ICON.lg} strokeWidth={2} className="text-[var(--accent)]" />
          <h3 className="text-sm font-semibold text-[var(--text)]">
            {t("admin.dashboardStats.nowPlaying")}
          </h3>
          {sessions.length > 0 && (
            <span className="text-xs font-medium px-2 py-0.5 rounded-full bg-[var(--accent)] text-[var(--filter-active-text)]">
              {sessions.length}
            </span>
          )}
        </div>
        <Link
          to="/admin/sessions"
          className="text-xs font-medium hover:underline text-[var(--accent)]"
        >
          {t("admin.dashboardStats.viewAll")}
        </Link>
      </div>
      {isLoading ? (
        <div className="flex justify-center py-6">
          <Spinner size="md" />
        </div>
      ) : isError ? (
        // "Nothing playing" and "I could not ask" look the same to an admin
        // watching for load.
        <FetchErrorState error={error} onRetry={() => void refetch()} />
      ) : sessions.length === 0 ? (
        <EmptyState
          icon={<Film size={40} strokeWidth={1.5} />}
          title={t("admin.dashboardStats.noActivePlayback")}
        />
      ) : (
        <div className="@container">
          <div className="grid grid-cols-1 @min-[600px]:grid-cols-2 gap-3">
            {sessions.map((s) => (
              <NowPlayingCard key={s.sessionId} session={s} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
