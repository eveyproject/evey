// playerStatus - a status listener that does not miss an early failure.
//
// expo-video drops the events nobody listens to yet. The native player starts
// preparing the moment it is created, during the render, while the screen
// attaches its listeners in an effect after the commit: on a slow device a
// source that fails within that gap left the player black for good, with no
// error screen and no beacon (measured: an unreadable SPS failed ~1 s after
// the request, and the screen kept reporting progress on a dead player).

export interface StatusEvent {
  status: string;
  error?: { message?: string } | null;
}

interface StatusPlayer {
  readonly status: string;
  addListener(event: "statusChange", listener: (ev: StatusEvent) => void): { remove(): void };
}

/**
 * Subscribes to statusChange and, when the player already failed before the
 * subscription, hands the handler that failure at once. Its message is lost
 * with the event, so it reads "failed before the screen listened".
 */
export function subscribePlayerStatus(player: StatusPlayer, onStatus: (ev: StatusEvent) => void): { remove(): void } {
  const sub = player.addListener("statusChange", onStatus);
  let status = "";
  try {
    status = player.status;
  } catch {
    /* released between creation and subscription */
  }
  if (status === "error") {
    onStatus({ status: "error", error: { message: EARLY_FAILURE } });
  }
  return sub;
}

export const EARLY_FAILURE = "player failed before the screen listened (error event missed)";
