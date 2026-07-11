// Canonical list of homepage feed types renderable by the web client.
// Backend may return additional types (legacy/experimental) — those are
// filtered out of the Customize panel since toggling them has no visual effect.

const KNOWN_FEED_TYPES = [
  "resume_playback",
  "next_episode",
  "collections",
  "recommended",
  "most_popular",
] as const;

export type KnownFeedType = (typeof KNOWN_FEED_TYPES)[number];

export const FEED_LABEL_KEYS: Record<KnownFeedType, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  next_episode: "common.feeds.labels.next_up",
  collections: "common.feeds.labels.collections",
  recommended: "common.feeds.labels.recommended",
  most_popular: "common.feeds.labels.most_popular",
};

export function isKnownFeedType(ft: string): ft is KnownFeedType {
  return (KNOWN_FEED_TYPES as readonly string[]).includes(ft);
}

// Plugin home-page sections are encoded as `plugin:<plugin_id>` so the
// customise-home menu treats them like library entries: each plugin slot
// has its own row, can be toggled / reordered.
export function isPluginFeedType(ft: string): boolean {
  return ft.startsWith("plugin:");
}

export function pluginIdFromFeedType(ft: string): string {
  return isPluginFeedType(ft) ? ft.slice("plugin:".length) : "";
}

export function pluginFeedType(pluginId: string): string {
  return "plugin:" + pluginId;
}
