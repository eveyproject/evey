import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";
import { toast } from "@evey/store";

const BTN_BASE =
  "inline-flex items-center justify-center rounded-[var(--radius-md)] font-medium transition-colors-fast bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]";

// Matches Button's own scale so the media-detail action row stays homogeneous.
const BTN_SIZE = { md: "gap-2 px-4 py-2 text-sm", lg: "gap-2.5 px-5 py-2.5 text-lg" } as const;
const ICON_SIZE = { md: 16, lg: 20 } as const;

interface DownloadButtonProps {
  mediaId: string;
  /** Filename hint for the browser (the <a download> attribute). */
  title?: string;
  /**
   * Token authorizing the download - a stream token scoped to this media.
   * Used when the caller already holds one (the player's single-file flow).
   */
  token?: string;
  /**
   * Mint a media-scoped stream token at click time instead of embedding a
   * pre-existing one. This is the container-archive path (season/series
   * ZIP): the account JWT must never ride in a URL (reverse-proxy logs,
   * browser history), and the download endpoint accepts a stream token
   * minted for the container id just fine.
   */
  mintToken?: () => Promise<string>;
  /** Label override, e.g. "Download season". Defaults to player.download. */
  label?: string;
  size?: keyof typeof BTN_SIZE;
}

function downloadURL(mediaId: string, token: string): string {
  return `/api/v1/media/${mediaId}/download?token=${encodeURIComponent(token)}`;
}

export function DownloadButton({ mediaId, title, token, mintToken, label, size = "md" }: DownloadButtonProps) {
  const { t } = useTranslation();
  const [minting, setMinting] = useState(false);
  const btnClass = `${BTN_BASE} ${BTN_SIZE[size]}`;
  if (!token && !mintToken) return null;

  if (token) {
    return (
      <a href={downloadURL(mediaId, token)} download={title} data-testid="download-btn" className={btnClass}>
        <Download size={ICON_SIZE[size]} />
        {label ?? t("player.download")}
      </a>
    );
  }

  const handleMintedDownload = async (e: React.MouseEvent) => {
    e.preventDefault();
    if (minting) return;
    setMinting(true);
    try {
      const tok = await mintToken!();
      const a = document.createElement("a");
      a.href = downloadURL(mediaId, tok);
      if (title) a.download = title;
      document.body.appendChild(a);
      a.click();
      a.remove();
    } catch {
      toast.error(t("common.error"));
    } finally {
      setMinting(false);
    }
  };

  return (
    <a href="#" onClick={handleMintedDownload} aria-disabled={minting} data-testid="download-btn" className={btnClass}>
      <Download size={ICON_SIZE[size]} />
      {label ?? t("player.download")}
    </a>
  );
}
