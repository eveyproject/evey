import { useEffect } from "react";
import { router } from "expo-router";
import { useAuthStore } from "@evey/store";
import { acknowledgeReady, isCastConnectAvailable, onCastLoad, publishSessionState, startCastReceiver } from "@evey/cast-tv";
import { getServerUrl } from "../lib/server-config";

/**
 * Makes this television a Cast Connect target: a cast from the web or the
 * phone OPENS Evey TV and plays through its own player, so the remote drives
 * the real thing - tracks, chapters, menus - instead of a picture the Web
 * Receiver can only start and stop.
 *
 * Two jobs, both small on purpose:
 *
 *   1. mirror the session facts the native LaunchRequestChecker reads before
 *      accepting a cast. It runs before React Native has booted on a cold
 *      launch, so it cannot ask us then - it reads the last state we wrote.
 *      Getting this WRONG is the 2026-09-19 failure: a television signed into
 *      the dev server answering a cast from prod. Refusing is not a loss, the
 *      Web Receiver takes it instead.
 *   2. turn an accepted LOAD into a navigation. The payload names a media id,
 *      never a URL: the player then negotiates its own stream, which is the
 *      whole point of handing the cast to the app.
 */
export function useCastConnectReceiver(): void {
  const accessToken = useAuthStore((s) => s.accessToken);
  const profileId = useAuthStore((s) => s.profileId);

  useEffect(() => {
    if (!isCastConnectAvailable()) return;
    startCastReceiver();
    const off = onCastLoad((payload) => {
      // `start` is what the player resumes at; the id/audio/subtitle params
      // are the ones the media page already passes, so the route contract is
      // unchanged.
      router.push({
        pathname: "/player/[id]",
        params: {
          id: payload.mediaId,
          ...(payload.positionSec > 0 ? { start: String(payload.positionSec) } : {}),
          ...(payload.audioTrack != null ? { audio: String(payload.audioTrack) } : {}),
          ...(payload.subtitleTrack != null ? { subtitle: String(payload.subtitleTrack) } : {}),
        },
      });
    });
    // Only now may a queued cold-boot LOAD be replayed: the listener above is
    // what will receive it.
    acknowledgeReady();
    return off;
  }, []);

  // The mirror. Re-written on every sign-in, sign-out and profile switch; the
  // server URL is read from SecureStore, which only JS can reach.
  useEffect(() => {
    if (!isCastConnectAvailable()) return;
    let cancelled = false;
    void (async () => {
      const serverOrigin = await getServerUrl().catch(() => null);
      if (cancelled) return;
      publishSessionState({
        signedIn: !!accessToken && !!serverOrigin,
        serverOrigin,
        userId: userIdFromToken(accessToken),
        profileId,
      });
    })();
    return () => { cancelled = true; };
  }, [accessToken, profileId]);
}

// The account the TV is signed in as, read from the access token's payload -
// the same field the rest of the app reads. Returns "" rather than throwing
// on a malformed token: an unknown account simply never contradicts the
// sender's, and the origin check still stands.
function userIdFromToken(token: string | null | undefined): string {
  if (!token) return "";
  try {
    const part = token.split(".")[1];
    if (!part) return "";
    const payload = JSON.parse(atob(part.replace(/-/g, "+").replace(/_/g, "/")));
    return typeof payload.sub === "string" ? payload.sub : typeof payload.user_id === "string" ? payload.user_id : "";
  } catch {
    return "";
  }
}
