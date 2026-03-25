import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";

interface DownloadButtonProps {
  mediaId: string;
  title?: string;
  streamToken?: string;
}

export function DownloadButton({ mediaId, title, streamToken }: DownloadButtonProps) {
  const { t } = useTranslation();
  const url = `/api/v1/media/${mediaId}/download${streamToken ? `?token=${encodeURIComponent(streamToken)}` : ""}`;

  return (
    <a
      href={url}
      download={title}
      data-testid="download-btn"
      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
    >
      <Download size={16} />
      {t("player.download")}
    </a>
  );
}
