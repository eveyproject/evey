import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import { useMediaSegments } from "@evey/client";
import {
  SKIPPABLE_SEGMENTS,
  SKIP_LABEL_KEY,
  activeSkippable,
  skippableCandidates,
} from "@evey/client";
import { Button } from "../components/ui";
import { FocusRoot } from "../tv/focus";

/**
 * The button the television shows while the playhead sits inside an intro, a
 * recap, the credits or a preview, with the admin's auto-skip applied to an
 * intro only - one jump per segment, so seeking back into it is the viewer
 * choosing to watch it.
 */
export function SkipSegmentButton({ mediaId, currentTime, onSkip, onNextEpisode }: {
  mediaId: string;
  currentTime: number;
  onSkip: (toSec: number) => void;
  onNextEpisode?: () => void;
}) {
  const { t } = useTranslation();
  const { data } = useMediaSegments(mediaId, [...SKIPPABLE_SEGMENTS]);
  const resolved = activeSkippable(skippableCandidates(data?.segments), currentTime);
  const autoSkip = data?.introAutoSkip === true;

  const autoSkipped = useRef<Set<number>>(new Set());
  const lastMedia = useRef(mediaId);
  if (lastMedia.current !== mediaId) { lastMedia.current = mediaId; autoSkipped.current = new Set(); }
  const autoTarget = autoSkip && resolved?.type === "intro" && !autoSkipped.current.has(resolved.startSec) ? resolved : null;
  const onSkipRef = useRef(onSkip);
  onSkipRef.current = onSkip;
  useEffect(() => {
    if (!autoTarget) return;
    autoSkipped.current.add(autoTarget.startSec);
    onSkipRef.current(autoTarget.endSec);
  }, [autoTarget]);

  if (!resolved || autoTarget) return null;
  return (
    <FocusRoot className="skip-segment">
      <Button variant="primary" onPress={() => onSkip(resolved.endSec)}>{t(SKIP_LABEL_KEY[resolved.type])}</Button>
      {resolved.type === "end_credits" && onNextEpisode ? (
        <Button onPress={onNextEpisode}>{t("player.controls.nextEpisode", "Next episode")}</Button>
      ) : null}
    </FocusRoot>
  );
}
