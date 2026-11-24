import type React from "react";
import { useTranslation } from "react-i18next";
import { AlertTriangle, Film, MonitorPlay, Volume2 } from "lucide-react";
import { describeSession, type ServedView, type V1ActiveSession } from "@evey/api";
import { Badge } from "@/components/ui/Badge";

// What a session IS: the bytes the server sent per track, and what the
// player says it decodes (@evey/api describeSession, shared with the phone's
// card and the player's own stream panel). Nothing here is read from a
// verdict; when the two witnesses disagree the card says so.

const BACKEND_KEYS: Record<string, string> = {
  software: "admin.sessions.track.backend.software",
  qsv: "admin.sessions.track.backend.qsv",
  vaapi: "admin.sessions.track.backend.vaapi",
  nvenc: "admin.sessions.track.backend.nvenc",
  rkmpp: "admin.sessions.track.backend.rkmpp",
};

function codecDetail(l: ServedView): string {
  const codec = l.sourceCodec ? `${l.sourceCodec}→${l.codec}` : l.codec;
  return [codec, l.height > 0 ? `${l.height}p` : ""].filter(Boolean).join(", ");
}

export function SessionTrackBadges({ session }: { session: V1ActiveSession }) {
  const { t } = useTranslation();
  const r = describeSession(session);
  if (!r.video && !r.audio && !r.played) {
    return (
      <div className="text-xs text-[var(--text-muted)] mt-1.5" data-testid="session-track-badges">
        {t("admin.sessions.track.waiting")}
      </div>
    );
  }

  const served = (id: "video" | "audio", l: ServedView) => {
    const icon: React.ReactNode = id === "video" ? <Film size={12} aria-hidden="true" /> : <Volume2 size={12} aria-hidden="true" />;
    const file = l.engine === "file";
    const label = file
      ? t("admin.sessions.track.fromFile")
      : t(`admin.sessions.track.${id}${l.copy ? "Copied" : "Transcoded"}`);
    const track = id === "audio" ? (l.track >= 0 ? t("admin.sessions.track.trackN", { n: l.track + 1 }) : file ? t("admin.sessions.track.allTracks") : "") : "";
    const detail = codecDetail(l);
    const engine = file ? "" : t(`admin.sessions.track.engine.${l.engine}`, { defaultValue: l.engine });
    const backend = l.backend ? t(BACKEND_KEYS[l.backend] ?? "", { defaultValue: l.backend }) : "";
    const servedBy = [engine, backend].filter(Boolean).join(" ");
    return (
      <div key={id} className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] min-w-0" data-testid={`served-${id}`}>
        <span>{icon}</span>
        <span className="text-[var(--text)]">{t(`admin.sessions.track.${id}`)}:</span>
        {track && <span className="truncate">{track} ·</span>}
        <Badge variant={l.copy ? "success" : "warning"}>{label}</Badge>
        {detail && <span className="font-mono truncate">({detail})</span>}
        {servedBy && <span className="opacity-70 truncate">· {servedBy}</span>}
      </div>
    );
  };

  const p = r.played;
  const playedParts = p
    ? [
        p.audioTrack >= 0 ? t("admin.sessions.track.trackN", { n: p.audioTrack + 1 }) : "",
        p.audioCodec,
        p.decoder === "wasm" ? t("admin.sessions.track.decoderWasm") : p.decoder === "native" ? t("admin.sessions.track.decoderNative") : "",
      ].filter(Boolean)
    : [];

  return (
    <div className="flex flex-col gap-1 mt-1.5" data-testid="session-track-badges">
      {r.video && served("video", r.video)}
      {r.audio && served("audio", r.audio)}
      {playedParts.length > 0 && (
        <div className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] min-w-0" data-testid="played">
          <MonitorPlay size={12} aria-hidden="true" />
          <span className="text-[var(--text)]">{t("admin.sessions.track.player")}:</span>
          <span className="truncate">{playedParts.join(" · ")}</span>
        </div>
      )}
      {r.trackMismatch && (
        <div className="flex items-center gap-1.5 text-xs text-[var(--color-warning)]" data-testid="mismatch-track">
          <AlertTriangle size={12} aria-hidden="true" />
          {t("admin.sessions.track.mismatchTrack", { played: r.trackMismatch.played + 1, served: r.trackMismatch.served + 1 })}
        </div>
      )}
      {r.codecMismatch && (
        <div className="flex items-center gap-1.5 text-xs text-[var(--color-warning)]" data-testid="mismatch-codec">
          <AlertTriangle size={12} aria-hidden="true" />
          {t("admin.sessions.track.mismatchCodec", r.codecMismatch)}
        </div>
      )}
    </div>
  );
}
