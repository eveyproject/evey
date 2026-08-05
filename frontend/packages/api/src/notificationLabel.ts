// The notification label, worded by the CLIENT.
//
// The server used to render it whole, in English, and store it:
// "new_series:Lioness (1 season, 1 episode)". Clients translated the prefix and
// copied the parenthesis through untouched, so a French TV displayed
// "Nouvelle série : Lioness (1 season, 1 episode)" - English in the middle of a
// French sentence, on an app that ships in six languages.
//
// The server now also sends the notification AS DATA: `subject` (the bare name)
// plus `seasonCount` / `episodeCount`. When they are there, the whole sentence
// is built here, in the viewer's language. When they are not - rows written
// before the change, which are deliberately NOT backfilled - the old parsing of
// `title` still applies, unchanged.
//
// This lived in three copies (web notificationUtils, mobile and TV
// notifications screens), each with its own TITLE_KEYS map.

const TITLE_KEYS: Record<string, string> = {
  new_movie: "common.notifications.newMovie",
  new_episode: "common.notifications.newEpisode",
  new_season: "common.notifications.newSeason",
  new_series: "common.notifications.newSeries",
  new_album: "common.notifications.newAlbum",
  backup_failed: "common.notifications.backupFailed",
};

export interface NotificationLabelInput {
  type?: string;
  title?: string;
  subject?: string;
  seasonCount?: number;
  episodeCount?: number;
}

type Translate = (key: string, opts?: Record<string, unknown>) => string;

/** The legacy path: split "type:name" and translate the prefix. */
function fromStoredTitle(t: Translate, raw: string | undefined): string {
  if (!raw) return "";
  const colonIdx = raw.indexOf(":");
  if (colonIdx === -1) return raw;
  const i18nKey = TITLE_KEYS[raw.slice(0, colonIdx)];
  if (!i18nKey) return raw;
  return t(i18nKey, { name: raw.slice(colonIdx + 1) });
}

export function notificationLabel(t: Translate, n: NotificationLabelInput): string {
  const subject = n.subject?.trim();
  if (!subject) return fromStoredTitle(t, n.title);

  // The type prefix still comes from the stored title: it is what the server
  // decided this notification IS, and it is already a stable machine value.
  const rawType = n.title?.includes(":") ? n.title.slice(0, n.title.indexOf(":")) : n.type;
  const i18nKey = TITLE_KEYS[rawType ?? ""];
  if (!i18nKey) return fromStoredTitle(t, n.title);

  const head = t(i18nKey, { name: subject });
  const counts: string[] = [];
  if (n.seasonCount && n.seasonCount > 0) {
    counts.push(t("common.notifications.seasonCount", { count: n.seasonCount }));
  }
  if (n.episodeCount && n.episodeCount > 0) {
    counts.push(t("common.notifications.episodeCount", { count: n.episodeCount }));
  }
  return counts.length > 0 ? `${head} (${counts.join(", ")})` : head;
}
