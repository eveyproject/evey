// The notification label, worded by the CLIENT.
//
// The server used to render it whole, in English, and store it:
// "new_series:Some Series (1 season, 1 episode)". Clients translated the prefix and
// copied the parenthesis through untouched, so a French TV displayed
// "Nouvelle série : Some Series (1 season, 1 episode)" - English in the middle of a
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
  media_request: "common.notifications.mediaRequest",
  request_answered: "common.notifications.requestAnswered",
};

// The English labels the server stored for media request notifications before
// it sent a subject. Rows written then still exist in every queue; stripping
// the known prefix recovers the title they are about, so they translate too.
const LEGACY_PREFIXES: Record<string, string> = {
  media_request: "New media request: ",
  request_answered: "Your media request was updated: ",
};

function legacySubject(n: NotificationLabelInput): string | undefined {
  const prefix = LEGACY_PREFIXES[n.type ?? ""];
  if (!prefix || !n.title?.startsWith(prefix)) return undefined;
  const rest = n.title.slice(prefix.length).trim();
  return rest || undefined;
}

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
  const subject = n.subject?.trim() || legacySubject(n);
  if (!subject) return fromStoredTitle(t, n.title);

  // The type prefix comes from the stored title where the server wrote one
  // ("new_series:..."): it is what the server decided this notification IS,
  // and it is already a stable machine value. Media request titles carry an
  // English sentence instead, so their type field is what names them.
  const titleType = n.title?.includes(":") ? n.title.slice(0, n.title.indexOf(":")) : undefined;
  const rawType = titleType && TITLE_KEYS[titleType] ? titleType : n.type;
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

// The media type a request is about, as the server stores it in Message.
const MEDIA_TYPE_KEYS: Record<string, string> = {
  movie: "requests.typeMovie",
  tv: "requests.typeSeries",
  music: "requests.typeMusic",
};

const STATUS_KEYS: Record<string, string> = {
  approved: "requests.statusApproved",
  rejected: "requests.statusRejected",
  later: "requests.statusLater",
};

// "Status: approved - note": the shape the server writes for an answered
// request. The status word is ours to translate; the note is the operator's,
// kept verbatim (it may itself contain " - ").
const ANSWERED_MESSAGE = /^Status:\s*(approved|rejected|later)(?:\s+-\s*([\s\S]*))?$/;

/**
 * The notification's second line, worded in the viewer's language where the
 * server stored a machine value: the media type of a new request ("movie"), or
 * the status of an answered one ("Status: approved - note"). Anything else is
 * returned as stored.
 */
export function notificationMessage(t: Translate, n: { type?: string; message?: string }): string {
  const message = n.message ?? "";
  if (n.type === "media_request") {
    const key = MEDIA_TYPE_KEYS[message.trim()];
    return key ? t(key) : message;
  }
  if (n.type === "request_answered") {
    const m = ANSWERED_MESSAGE.exec(message.trim());
    if (!m) return message;
    const status = t(STATUS_KEYS[m[1]]);
    return m[2] ? `${status} - ${m[2]}` : status;
  }
  return message;
}
