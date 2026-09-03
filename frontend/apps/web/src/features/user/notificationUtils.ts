import { useTranslation } from "react-i18next";
import { notificationLabel, type NotificationLabelInput } from "@evey/api";

// Kept as this module's entry point; the wording lives in @evey/api so the
// three apps share one implementation. Pass the whole notification: the
// subject and counts are what let the sentence be built in the viewer's
// language instead of copying the server's English through.
export function translateNotificationTitle(
  t: (k: string, opts?: Record<string, unknown>) => string,
  n: NotificationLabelInput | string | undefined,
): string {
  return notificationLabel(t, typeof n === "string" ? { title: n } : (n ?? {}));
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

// notificationUrl is where a notification leads: a media request pages an
// admin, so the actionable place is the moderation queue (those never carry a
// mediaItemId - the title is not in the library yet, that is the whole point);
// anything about a title opens it.
export function notificationUrl(n: { type?: string; mediaItemId?: string }): string | undefined {
  if (n.type === "media_request") return "/admin/media-requests";
  if (n.mediaItemId) return `/media/${n.mediaItemId}`;
  return undefined;
}
