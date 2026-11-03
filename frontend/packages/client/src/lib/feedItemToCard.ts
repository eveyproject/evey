import type { V1MediaItem } from "@evey/api";

/** The fields of a feed item (recommended, similar, recently added) a card reads. */
export interface FeedItemLike {
  mediaItemId?: string;
  title?: string;
  type?: string;
  year?: number;
  primaryBlurhash?: string;
}

/** A feed item projected onto the card shape every app renders. */
export type FeedCard = V1MediaItem & { id: string };

// The feed answers with its own shape (mediaItemId, primaryBlurhash); the
// card components read a MediaItem. One projection for every rail and app:
// the per-screen copies had drifted, and the similar rail dropped the year
// and the blurhash the server writes.
export function feedItemToCard(fi: FeedItemLike): FeedCard {
  return {
    id: fi.mediaItemId ?? "",
    title: fi.title,
    type: fi.type,
    year: fi.year || undefined,
    artwork: fi.primaryBlurhash ? [{ type: "ARTWORK_TYPE_PRIMARY", blurhash: fi.primaryBlurhash }] : undefined,
  } as FeedCard;
}

/** Every feed item that names a title, as cards. */
export function feedItemsToCards(items: FeedItemLike[] | undefined): FeedCard[] {
  return (items ?? []).filter((fi) => !!fi.mediaItemId).map(feedItemToCard);
}
