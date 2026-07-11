import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";
import { toast } from "@evey/store";

const BTN_CLASS =
  "inline-flex items-center justify-center gap-2 px-4 py-2 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]";

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
}

function downloadURL(mediaId: string, token: string): string {
  return `/api/v1/media/${mediaId}/download?token=${encodeURIComponent(token)}`;
}

export function DownloadButton({ mediaId, title, token, mintToken, label }: DownloadButtonProps) {
  const { t } = useTranslation();
  const [minting, setMinting] = useState(false);
  if (!token && !mintToken) return null;

  if (token) {
    return (
      <a href={downloadURL(mediaId, token)} download={title} data-testid="download-btn" className={BTN_CLASS}>
        <Download size={16} />
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
    <a href="#" onClick={handleMintedDownload} aria-disabled={minting} data-testid="download-btn" className={BTN_CLASS}>
      <Download size={16} />
      {label ?? t("player.download")}
    </a>
  );
}
