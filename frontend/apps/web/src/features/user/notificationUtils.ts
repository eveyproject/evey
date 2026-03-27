import { useTranslation } from "react-i18next";

const TITLE_KEYS: Record<string, string> = {
  new_movie: "common.notifications.newMovie",
  new_episode: "common.notifications.newEpisode",
  new_season: "common.notifications.newSeason",
  new_series: "common.notifications.newSeries",
  new_album: "common.notifications.newAlbum",
  backup_failed: "common.notifications.backupFailed",
};

export function translateNotificationTitle(
  t: (k: string, opts?: Record<string, unknown>) => string,
  raw: string | undefined,
): string {
  if (!raw) return "";
  const colonIdx = raw.indexOf(":");
  if (colonIdx === -1) return raw;
  const key = raw.slice(0, colonIdx);
  const i18nKey = TITLE_KEYS[key];
  if (!i18nKey) return raw;
  const name = raw.slice(colonIdx + 1);
  return t(i18nKey, { name });
}

export function useRelativeTime() {
  const { t } = useTranslation();
  return (iso: string | undefined): string => {
    if (!iso) return "";
    const diff = Date.now() - new Date(iso).getTime();
    const mins = Math.floor(diff / 60000);
    if (mins < 1) return t("common.notifications.justNow");
    if (mins < 60) return t("common.notifications.minutesAgo", { count: mins });
    const hours = Math.floor(mins / 60);
    if (hours < 24) return t("common.notifications.hoursAgo", { count: hours });
    const days = Math.floor(hours / 24);
    if (days < 7) return t("common.notifications.daysAgo", { count: days });
    return new Date(iso).toLocaleDateString(undefined, { month: "short", day: "numeric" });
  };
}
