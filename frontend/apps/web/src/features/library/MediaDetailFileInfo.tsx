import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { StreamList } from "./StreamList";
import { MediaTechSummary } from "./MediaTechSummary";
import { formatBytes, formatDurationMs } from "@evey/api";

export function MediaDetailFileInfo({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const [fileInfoOpen, setFileInfoOpen] = useState(false);
  const duration = formatDurationMs(item.durationMs);
  // Overall bitrate: use the stored value, but fall back to size×8/duration when
  // it's 0/absent - MKV containers (and the prober) often don't carry an overall
  // bitrate, which otherwise renders a useless "0 kbps".
  const bitrateKbps = (() => {
    const stored = item.bitrate ? Number(item.bitrate) : 0;
    if (stored > 0) return Math.round(stored / 1000);
    const size = Number(item.fileSize ?? 0);
    const durSec = Number(item.durationMs ?? 0) / 1000;
    return size > 0 && durSec > 0 ? Math.round((size * 8) / durSec / 1000) : 0;
  })();

  // A container (series/season) has no file of its own. The API emits proto
  // zero values (EmitUnpopulated), so fileSize/durationMs arrive as the STRING
  // "0" - which is truthy in JS, so a naive `!item.fileSize` check let the
  // disclosure render with a useless "0 B". Gate on REAL presence instead.
  const hasFile =
    !!item.filePath ||
    Number(item.fileSize ?? 0) > 0 ||
    !!item.containerFormat ||
    (item.streams?.length ?? 0) > 0;
  if (!hasFile) {
    return null;
  }

  return (
    <div className="mt-10">
      {/* The technical summary is always visible: it was the whole point of the
          disclosure for most viewers, and keeping it collapsed meant nobody knew
          what they were about to play. The exhaustive per-track list stays
          behind the toggle. */}
      <div className="mb-3">
        <MediaTechSummary item={item} />
      </div>
      <button
        className="flex items-center gap-2 text-sm font-semibold mb-3 text-[var(--text-muted)]"
        onClick={() => setFileInfoOpen((v) => !v)}
        aria-expanded={fileInfoOpen}
        aria-label={t("media.detail.fileInfo")}
      >
        <ChevronRight
          className="transition-transform-fast"
          size={16}
          style={{ transform: fileInfoOpen ? "rotate(90deg)" : "rotate(0deg)" }}
         aria-hidden="true" />
        {t("media.detail.fileInfo")}
      </button>
      {fileInfoOpen && (
        <div className="space-y-2">
          {item.filePath && (
            <div className="text-xs font-mono truncate text-[var(--text-muted)]">{item.filePath}</div>
          )}
          <div className="flex gap-4 flex-wrap">
            {item.fileSize && <Badge variant="default">{formatBytes(item.fileSize)}</Badge>}
            {item.containerFormat && <Badge variant="default">{item.containerFormat.toUpperCase()}</Badge>}
            {bitrateKbps > 0 && <Badge variant="default">{bitrateKbps} kbps</Badge>}
            {duration && <Badge variant="default">{duration}</Badge>}
          </div>
          {item.streams && item.streams.length > 0 && (
            <div className="mt-4"><StreamList streams={item.streams} /></div>
          )}
        </div>
      )}
    </div>
  );
}
