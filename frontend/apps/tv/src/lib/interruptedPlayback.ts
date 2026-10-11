import * as SecureStore from "expo-secure-store";
import { trackPickParams, type TrackPick } from "@evey/api";

// A playback the system cut short. The player keeps a mark while a title plays
// and wipes it when the viewer leaves; a mark still there at the next launch
// means the process died under the player (a Play Services update, the memory
// killer), and the home offers to carry on once.

const KEY = "evey.interruptedPlayback";
/** Past this, the evening is over: the home's own row is enough. */
export const INTERRUPTED_MAX_AGE_MS = 12 * 60 * 60 * 1000;

export interface InterruptedPlayback {
  mediaId: string;
  profileId: string;
  title: string;
  positionMs: number;
  pick: TrackPick;
  /** When the mark was last written, epoch ms. */
  at: number;
}

export async function markPlaying(mark: InterruptedPlayback): Promise<void> {
  try {
    await SecureStore.setItemAsync(KEY, JSON.stringify(mark));
  } catch {
    // A lost mark only loses the offer.
  }
}

export async function clearPlaying(): Promise<void> {
  try {
    await SecureStore.deleteItemAsync(KEY);
  } catch {
    // Same: at worst one offer too many, for a title the viewer had left.
  }
}

/**
 * The interrupted playback to offer to this profile, or null. The mark is
 * consumed whatever the answer: one offer per interruption, never a loop.
 */
export async function takeInterrupted(profileId: string, now = Date.now()): Promise<InterruptedPlayback | null> {
  let raw: string | null = null;
  try {
    raw = await SecureStore.getItemAsync(KEY);
  } catch {
    return null;
  }
  if (!raw) return null;
  await clearPlaying();
  let mark: Partial<InterruptedPlayback>;
  try {
    mark = JSON.parse(raw) as Partial<InterruptedPlayback>;
  } catch {
    return null;
  }
  if (typeof mark.mediaId !== "string" || !mark.mediaId || mark.profileId !== profileId) return null;
  if (typeof mark.at !== "number" || now - mark.at > INTERRUPTED_MAX_AGE_MS || mark.at > now) return null;
  return {
    mediaId: mark.mediaId,
    profileId,
    title: typeof mark.title === "string" ? mark.title : "",
    positionMs: typeof mark.positionMs === "number" && mark.positionMs > 0 ? mark.positionMs : 0,
    pick: mark.pick ?? {},
    at: mark.at,
  };
}

/** The player route that carries on: the server's progress places it, the pick restores the tracks. */
export function resumeRoute(mark: InterruptedPlayback): { pathname: "/player/[id]"; params: Record<string, string> } {
  return { pathname: "/player/[id]", params: { id: mark.mediaId, ...trackPickParams(mark.pick) } };
}
