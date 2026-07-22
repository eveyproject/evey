import { useState } from "react";
import { useTranslation } from "react-i18next";
import { Download } from "lucide-react";
import { toast } from "@evey/store";
import { ACTION_ICON_BOX } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";

const BTN_BASE =
  "inline-flex items-center justify-center rounded-[var(--radius-md)] font-medium transition-colors-fast bg-transparent hover:bg-[var(--surface-raised)] text-[var(--text-muted)]";

// Mirrors Button's own scale so the media-detail action row stays homogeneous
// - "lg" here and Button's "lg" must render the same 42px box.
// "icon" is the label-less form used in episode rows: with a label of "" the
// text sizes produced a lopsided 48x32 box, so it takes the square icon box
// the other icon-only controls use instead.
const BTN_SIZE = {
  md: "gap-2 px-4 py-2 text-sm",
  lg: "gap-2 px-4.5 py-2.5 text-base leading-snug",
  icon: `${ACTION_ICON_BOX} rounded-full`,
} as const;
const ICON_SIZE = { md: ICON.md, lg: ICON.md, icon: ICON.xl } as const;

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
  // The icon form is unlabelled by construction; the accessible name comes
  // from the title/aria-label instead of a visible string.
  const text = size === "icon" ? null : (label ?? t("player.download"));
  const iconOnlyProps = size === "icon"
    ? { title: label || t("player.download"), "aria-label": label || t("player.download") }
    : {};
  if (!token && !mintToken) return null;

  if (token) {
    return (
      <a href={downloadURL(mediaId, token)} download={title} data-testid="download-btn" className={btnClass} {...iconOnlyProps}>
        <Download size={ICON_SIZE[size]} />
        {text}
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
    <a href="#" onClick={handleMintedDownload} aria-disabled={minting} data-testid="download-btn" className={btnClass} {...iconOnlyProps}>
      <Download size={ICON_SIZE[size]} />
      {text}
    </a>
  );
}
