import { useCallback, useState } from "react";
import { Cast } from "lucide-react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { playbackServiceGetStreamInfo } from "@evey/api";
import { castProfileFor, castStreamInfoParams } from "@evey/media-caps/cast";
import { userIdFromAccessToken } from "@evey/media-caps/launch";
import { useAuthStore } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { ICON } from "@/constants/iconSizes";
import { useCasting, type CastTarget } from "@/hooks/useCasting";
import { useStreamToken } from "@/hooks/useStreamToken";
import { planCastLoad } from "./hooks/hlsPlayerUtils";

export interface CastMediaButtonProps {
  mediaId: string;
  title?: string;
  /** Resume position in ms, as the detail page already knows it. */
  resumeMs?: number;
  /** Audio track chosen on the page (absolute stream index), if any. */
  audioTrack?: number;
  /** Parental gate: the PIN must be answered BEFORE the stream leaves for a
   *  television, exactly as it is before the local player opens. Returns true
   *  when the gate took over and the cast must not start. */
  gate?: () => boolean;
}

// Casting used to start from inside the player only: you opened the film on
// the computer, waited for it to play, then handed it over. From the media
// page the intent is plain - "play this on the TV" - and there is no reason
// to make the browser decode a frame first.
//
// The stream is negotiated for the RECEIVER, never for this browser: the same
// castProfileFor / castStreamInfoParams / planCastLoad the player uses, so
// there is one negotiation in the codebase and this is not a second opinion.
export function CastMediaButton({ mediaId, title, resumeMs, audioTrack, gate }: CastMediaButtonProps) {
  const { t } = useTranslation();
  const { castSdkReady, connected, deviceName, startCast, stopCast } = useCasting();
  const { fetchStreamToken } = useStreamToken();
  // Identity for the Cast Connect launch credentials: a television signed in
  // as someone else must be able to refuse this cast.
  const profileId = useAuthStore((s) => s.profileId);
  const userId = userIdFromAccessToken(useAuthStore((s) => s.accessToken));
  const [busy, setBusy] = useState(false);

  const resolveCastUrl = useCallback(async (target: CastTarget): Promise<{ url: string; castResumeSec: number }> => {
    // A cast token, not the session one: the receiver buffers minutes ahead
    // and a 5-minute sliding token dies mid-film.
    const token = await fetchStreamToken(mediaId, { cast: true });
    const profile = castProfileFor(target.kind === "sdk" ? target.caps : null, target.kind === "server" ? target.model : null);
    const verdict = await playbackServiceGetStreamInfo(mediaId, castStreamInfoParams(profile)).catch(() => null);
    const origin = localStorage.getItem("cast_base_url") || window.location.origin;
    const plan = planCastLoad(origin, mediaId, token, profile, verdict, audioTrack ?? 0, (resumeMs ?? 0) / 1000, false);
    return { url: plan.url, castResumeSec: plan.resumeSec };
  }, [fetchStreamToken, mediaId, audioTrack, resumeMs]);

  const onClick = useCallback(async () => {
    if (connected) {
      void stopCast();
      return;
    }
    if (gate?.()) return;
    setBusy(true);
    try {
      const outcome = await startCast(resolveCastUrl, title, {
        userId, profileId: profileId ?? undefined,
        // Lets an Evey TV app play this by id, with its own player.
        eveyMedia: { mediaId, audioTrack },
      });
      if (outcome === "no-devices") toast.error(t("player.cast.noDevices"));
      else if (outcome === "session-failed") toast.error(t("player.cast.connectFailed"));
    } finally {
      setBusy(false);
    }
  }, [connected, stopCast, gate, startCast, resolveCastUrl, title, t, userId, profileId, mediaId, audioTrack]);

  // No Cast path in this browser (Firefox, Safari, iOS): no button. The
  // server-brokered fallback inside startCast only exists for browsers that
  // got this far, and offering a control that can only fail is worse than
  // offering none.
  if (!castSdkReady) return null;

  return (
    <Button
      variant="secondary"
      size="lg"
      data-testid="cast-media-button"
      loading={busy}
      onClick={() => void onClick()}
      title={connected && deviceName ? `${t("player.controls.castingTo")} ${deviceName}` : undefined}
    >
      <Cast size={ICON.md} />
      {connected ? t("player.controls.stopCast") : t("player.controls.cast")}
    </Button>
  );
}
