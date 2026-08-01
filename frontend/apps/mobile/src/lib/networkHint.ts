// Network hints for downloads. Nothing here BLOCKS anything: a download on
// mobile data is the user's call, and Evey is often reached over a phone's own
// connection away from home. We only tell them what they are about to spend.
//
// This replaces an earlier gate that held the offline audio pass until the
// device was charging AND on Wi-Fi. That gate was wrong on both counts: the
// pass is pure local CPU over a file that is already on disk - it never opens
// a socket - so Wi-Fi had nothing to do with it, and making it wait for a
// charger only meant the user plugged in and saw nothing happen.
import * as Network from "expo-network";

/**
 * True when the current connection is one the user is likely paying for by the
 * megabyte. An unknown or unreadable network state reads as NOT metered: this
 * value only decides whether to add a line to a toast, so guessing "metered"
 * on a failed probe would nag people on their home Wi-Fi for nothing.
 */
export async function isMeteredNetwork(): Promise<boolean> {
  try {
    const state = await Network.getNetworkStateAsync();
    return state.type === Network.NetworkStateType.CELLULAR;
  } catch {
    return false;
  }
}
