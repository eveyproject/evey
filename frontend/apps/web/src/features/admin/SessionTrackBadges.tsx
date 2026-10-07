import type React from "react";
import { useTranslation } from "react-i18next";
import { Film, Volume2 } from "lucide-react";
import { sessionTrackLines, type V1ActiveSession } from "@evey/api";
import { Badge } from "@/components/ui/Badge";

// The per-track lines live in @evey/api (sessionTrackLines), shared with the
// phone's session card so the two never disagree on what serves a track.
export { sessionTrackLines };

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
  rkmpp: "admin.sessions.track.backend.rkmpp",
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
