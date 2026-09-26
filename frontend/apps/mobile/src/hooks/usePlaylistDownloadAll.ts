import { useCallback, useState } from "react";
import { router } from "expo-router";
import { useTranslation } from "react-i18next";
import { libraryServiceGetMediaItem } from "@evey/api";
import { toast } from "@evey/store";
import { useActiveProfile } from "./useActiveProfile";
import { startDownload, getDownloadPath } from "../lib/downloads";
import { buildDownloadAuthContext } from "../lib/downloadContext";
import { confirmDownloadOnMobileData } from "../lib/mobileDataGate";
import { downloadPlaylistItems } from "../lib/playlistDownload";

// Downloads a whole playlist, title by title (the web's ZIP is not playable
// offline). The downloader's profile rides every file for the offline
// parental gate: the profile the session runs as (useActiveProfile).
export function usePlaylistDownloadAll(mediaItemIds: string[]) {
  const { t } = useTranslation();
  const { profile: activeProfile } = useActiveProfile();
  const [downloading, setDownloading] = useState(false);

  const downloadAll = useCallback(async () => {
    if (mediaItemIds.length === 0 || downloading) return;
    if (!(await confirmDownloadOnMobileData(t))) return;
    setDownloading(true);
    toast.info(t("media.detail.downloadStarted"), { onPress: () => router.push("/downloads") });
    try {
      const r = await downloadPlaylistItems(mediaItemIds, {
        getItem: (id) => libraryServiceGetMediaItem(id),
        isDownloaded: async (id) => !!(await getDownloadPath(id)),
        start: (id, title, opts) => startDownload(id, title, undefined, opts),
        authContext: (item) => buildDownloadAuthContext(activeProfile, item),
      });
      const summary = t("media.playlist.downloadAllDone", { started: r.started, skipped: r.skipped, failed: r.failed });
      if (r.failed > 0) toast.error(summary);
      else toast.success(summary);
    } finally {
      setDownloading(false);
    }
  }, [mediaItemIds, downloading, t, activeProfile]);

  return { downloadAll, downloading };
}
