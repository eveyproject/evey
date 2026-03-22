import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";
import { useAuthStore } from "@evey/store";

interface DownloadButtonProps {
  mediaId: string;
  title?: string;
}

export function DownloadButton({ mediaId, title }: DownloadButtonProps) {
  const { t } = useTranslation();
  const token = useAuthStore((s) => s.accessToken);
  const url = `/api/v1/media/${mediaId}/download${token ? `?token=${encodeURIComponent(token)}` : ""}`;

  return (
    <a
      href={url}
      download={title}
      data-testid="download-btn"
      className="inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]"
    >
      <Download size={16} />
      {t("player.download")}
    </a>
  );
}
