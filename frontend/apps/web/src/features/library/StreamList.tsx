import { useTranslation } from "react-i18next";
import { languageDisplayName, type V1MediaStream } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { hdrLabel, resolutionLabel } from "./streamLabels";

interface StreamListProps {
  streams?: V1MediaStream[];
}

function StreamRow({ stream }: { stream: V1MediaStream }) {
  const { t, i18n } = useTranslation();
  const name = languageDisplayName(stream.language, i18n.language);
  // A track titled "French" already says it: no "French French".
  const language = name && name.toLowerCase() !== stream.title?.trim().toLowerCase() ? name : null;
  return (
    <div
      className="flex items-center gap-3 py-2 px-3 rounded-[var(--radius-sm)] bg-[var(--surface-raised)]"
    >
      {stream.codec && (
        <span className="text-xs font-mono uppercase text-[var(--text)]">
          {stream.codec}
        </span>
      )}
      {language && (
        <span className="text-xs text-[var(--text-muted)]" title={stream.language}>
          {language}
        </span>
      )}
      {stream.title && (
        <span className="text-xs flex-1 truncate text-[var(--text-muted)]">
          {stream.title}
        </span>
      )}
      {stream.isDefault && <Badge variant="info">{t("media.streams.default")}</Badge>}
      {stream.isForced && <Badge variant="warning">{t("media.streams.forced")}</Badge>}
      {stream.isExternal && <Badge variant="info">{t("media.streams.external")}</Badge>}
    </div>
  );
}

function VideoStreamRow({ stream }: { stream: V1MediaStream }) {
  const hdr = hdrLabel(stream);
  // The tier every other surface prints, then the exact frame for whoever
  // needs it: "1080p · 1920×818", not a bare "1920x818" beside a "720p" chip.
  const tier = resolutionLabel(stream);
  const frame = stream.width && stream.height ? `${stream.width}×${stream.height}` : null;
  const dims = tier && frame ? `${tier} · ${frame}` : tier ?? frame;
  const fps = stream.frameRate ? `${Number(stream.frameRate).toFixed(3).replace(/\.?0+$/, "")} fps` : null;
  const depth = stream.bitDepth ? `${stream.bitDepth} bit` : null;

  return (
    <div className="flex items-center gap-3 py-2 px-3 rounded-[var(--radius-sm)] bg-[var(--surface-raised)]">
      {stream.codec && (
        <span className="text-xs font-mono uppercase text-[var(--text)]">{stream.codec}</span>
      )}
      {dims && <span className="text-xs tabular-nums text-[var(--text-muted)]">{dims}</span>}
      {fps && <span className="text-xs tabular-nums text-[var(--text-muted)]">{fps}</span>}
      {depth && <span className="text-xs tabular-nums text-[var(--text-muted)]">{depth}</span>}
      {hdr && <Badge variant="info">{hdr}</Badge>}
      {stream.profile && (
        <span className="text-xs truncate text-[var(--text-muted)]">{stream.profile}</span>
      )}
    </div>
  );
}

export function StreamList({ streams }: StreamListProps) {
  const { t } = useTranslation();
  if (!streams || streams.length === 0) return null;

  const video = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_VIDEO");
  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");

  return (
    <div className="space-y-4">
      {/* Video was missing entirely from this list: it only ever filtered audio
          and subtitles, so the one track that decides whether a title transcodes
          was the one you couldn't see. */}
      {video.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-2 text-[var(--text-muted)]">
            {t("media.streams.video")}
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-1">
            {video.map((s) => (
              <VideoStreamRow key={s.index} stream={s} />
            ))}
          </div>
        </div>
      )}
      {audio.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-2 text-[var(--text-muted)]">
            {t("media.streams.audio")}
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-1">
            {audio.map((s) => (
              <StreamRow key={s.index} stream={s} />
            ))}
          </div>
        </div>
      )}
      {subtitles.length > 0 && (
        <div>
          <h3 className="text-sm font-semibold mb-2 text-[var(--text-muted)]">
            {t("media.streams.subtitles")}
          </h3>
          <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-1">
            {subtitles.map((s) => (
              <StreamRow key={s.index} stream={s} />
            ))}
          </div>
        </div>
      )}
    </div>
  );
}
