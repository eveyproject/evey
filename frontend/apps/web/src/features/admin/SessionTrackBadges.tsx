import type React from "react";
import { useTranslation } from "react-i18next";
import { Film, Volume2 } from "lucide-react";
import type { V1ActiveSession } from "@evey/api";
import { Badge } from "@/components/ui/Badge";

// What actually serves each track. The server records videoEngine /
// videoBackend / audioEngine when it renders the master; a session that
// predates that (or a direct play) falls back to the old guess: copy is the
// packager, transcode is ffmpeg.
export interface TrackLine {
  id: "video" | "audio";
  decision: string; // copy | transcode | wasm
  engine: string; // mkvgo | ffmpeg | client
  backend: string; // transcode only: software | qsv | vaapi | nvenc
  codec?: string;
  res?: string;
}

export function sessionTrackLines(session: V1ActiveSession): TrackLine[] {
  const vDec = (session.videoDecision ?? "").trim();
  const aDec = (session.audioDecision ?? "").trim();
  const guess = (decision: string) => (decision === "transcode" ? "ffmpeg" : "mkvgo");
  const lines: TrackLine[] = [];
  if (vDec) {
    lines.push({
      id: "video",
      decision: vDec,
      engine: session.videoEngine || guess(vDec),
      backend: vDec === "transcode" ? session.videoBackend ?? "" : "",
      codec: session.videoCodec,
      res: session.resolution,
    });
  }
  if (aDec) {
    lines.push({
      id: "audio",
      decision: aDec,
      engine: session.audioEngine || (aDec === "wasm" ? "client" : guess(aDec)),
      backend: "",
      codec: session.audioCodec,
    });
  }
  return lines;
}

const ENGINE_KEYS: Record<string, string> = {
  mkvgo: "admin.sessions.track.engine.mkvgo",
  ffmpeg: "admin.sessions.track.engine.ffmpeg",
  client: "admin.sessions.track.engine.client",
};

const BACKEND_KEYS: Record<string, string> = {
  software: "admin.sessions.track.backend.software",
  qsv: "admin.sessions.track.backend.qsv",
  vaapi: "admin.sessions.track.backend.vaapi",
  nvenc: "admin.sessions.track.backend.nvenc",
};

export function SessionTrackBadges({ session }: { session: V1ActiveSession }) {
  const { t } = useTranslation();
  const lines = sessionTrackLines(session);
  if (lines.length === 0) return null;

  return (
    <div className="flex flex-col gap-1 mt-1.5" data-testid="session-track-badges">
      {lines.map((l) => {
        const icon: React.ReactNode = l.id === "video" ? <Film size={12} aria-hidden="true" /> : <Volume2 size={12} aria-hidden="true" />;
        const isTranscode = l.decision === "transcode";
        const decLabel =
          l.decision === "wasm"
            ? t("admin.sessions.track.audioClientDecoded")
            : t(`admin.sessions.track.${l.id}${isTranscode ? "Transcoded" : "Copied"}`);
        const detail = [l.codec, l.res].filter(Boolean).join(", ");
        const engine = ENGINE_KEYS[l.engine] ? t(ENGINE_KEYS[l.engine]) : l.engine;
        const backend = l.backend ? (BACKEND_KEYS[l.backend] ? t(BACKEND_KEYS[l.backend]) : l.backend) : "";
        return (
          <div key={l.id} className="flex items-center gap-1.5 text-xs text-[var(--text-muted)] min-w-0">
            <span className="text-[var(--text-muted)]">{icon}</span>
            <span className="text-[var(--text)]">{t(`admin.sessions.track.${l.id}`)}:</span>
            <Badge variant={isTranscode ? "warning" : l.decision === "wasm" ? "info" : "success"}>{decLabel}</Badge>
            {detail && <span className="font-mono truncate">({detail})</span>}
            <span className="opacity-70 truncate">· {[engine, backend].filter(Boolean).join(" ")}</span>
          </div>
        );
      })}
    </div>
  );
}
