import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { Download } from "lucide-react";
import { ICON } from "@/constants/iconSizes";
import { startTokenDownload } from "@/utils/downloadToken";

export function DownloadTrackButton({ trackID }: { trackID: string }) {
  const { t } = useTranslation();
  async function download() {
    // Click-time mint: the account JWT must not ride in the URL (proxy logs,
    // browser history) - see utils/downloadToken.ts.
    try {
      await startTokenDownload(`/api/v1/music/tracks/${encodeURIComponent(trackID)}/download`, trackID);
    } catch {
      toast.error(t("common.error"));
    }
  }
  return (
    <button
      type="button"
      onClick={download}
      aria-label={t("media.music.downloadTrack")}
      title={t("media.music.downloadTrack")}
      className="bg-transparent border-none cursor-pointer text-[var(--text-muted)] hover:text-[var(--accent)] p-1 rounded mr-1"
    >
      <Download size={ICON.lg} />
    </button>
  );
}
