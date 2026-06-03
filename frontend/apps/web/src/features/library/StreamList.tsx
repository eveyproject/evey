import { useTranslation } from "react-i18next";
import type { V1MediaStream } from "@evey/api";
import { Badge } from "@/components/ui/Badge";

interface StreamListProps {
  streams?: V1MediaStream[];
}

function StreamRow({ stream }: { stream: V1MediaStream }) {
  const { t } = useTranslation();
  return (
    <div
      className="flex items-center gap-3 py-2 px-3 rounded-[var(--radius-sm)] bg-[var(--surface-raised)]"
    >
      {stream.codec && (
        <span className="text-xs font-mono uppercase text-[var(--text)]">
          {stream.codec}
        </span>
      )}
      {stream.language && (
        <span className="text-xs uppercase text-[var(--text-muted)]">
          {stream.language}
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

export function StreamList({ streams }: StreamListProps) {
  const { t } = useTranslation();
  if (!streams || streams.length === 0) return null;

  const audio = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_AUDIO");
  const subtitles = streams.filter((s) => s.type === "MEDIA_STREAM_TYPE_SUBTITLE");

  return (
    <div className="space-y-4">
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
