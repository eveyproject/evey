import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";

interface DownloadButtonProps {
  mediaId: string;
  /** Filename hint for the browser (the <a download> attribute). */
  title?: string;
  /**
   * Token authorizing the download. A stream token for single files
   * (movie/episode), or the access token for container archives
   * (season/series ZIP, which have no stream token of their own).
   */
  token?: string;
  /** Label override, e.g. "Download season". Defaults to player.download. */
  label?: string;
}

export function DownloadButton({ mediaId, title, token, label }: DownloadButtonProps) {
  const { t } = useTranslation();
  if (!token) return null;
  const url = `/api/v1/media/${mediaId}/download?token=${encodeURIComponent(token)}`;

  return (
    <a
      href={url}
      download={title}
      data-testid="download-btn"
      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
    >
      <Download size={16} />
      {label ?? t("player.download")}
    </a>
  );
}
