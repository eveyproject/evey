import { useState } from "react";
import { ChevronRight } from "lucide-react";
import { useTranslation } from "react-i18next";
import type { V1MediaItem } from "@evey/api";
import { Badge } from "@/components/ui/Badge";
import { StreamList } from "./StreamList";
import { formatDuration, formatFileSize } from "./mediaDetailUtils";

export function MediaDetailFileInfo({ item }: { item: V1MediaItem }) {
  const { t } = useTranslation();
  const [fileInfoOpen, setFileInfoOpen] = useState(false);
  const duration = formatDuration(item.durationMs);

  if (!item.filePath && !item.fileSize && !item.containerFormat && !(item.streams && item.streams.length > 0)) {
    return null;
  }

  return (
    <div className="mt-10">
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
            {item.fileSize && <Badge variant="default">{formatFileSize(item.fileSize)}</Badge>}
            {item.containerFormat && <Badge variant="default">{item.containerFormat.toUpperCase()}</Badge>}
            {item.bitrate && <Badge variant="default">{Math.round(parseInt(item.bitrate) / 1000)} kbps</Badge>}
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
