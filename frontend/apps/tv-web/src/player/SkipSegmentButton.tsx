import { useEffect, useRef } from "react";
import { useTranslation } from "react-i18next";
import {
  useMediaSegments,
  useChapterSkippables,
} from "@evey/client";
import {
  SKIPPABLE_SEGMENTS,
  SKIP_LABEL_KEY,
  activeSkippable,
  skippableCandidates,
} from "@evey/client";
import { Pressable } from "../components/ui";
import { FocusRoot } from "../tv/focus";

/**
 * The button the television shows while the playhead sits inside an intro, a
 * recap, the credits or a preview, with the admin's auto-skip applied to an
 * intro only - one jump per segment, so seeking back into it is the viewer
 * choosing to watch it.
 */
export function SkipSegmentButton({ mediaId, filePath, currentTime, onSkip, onNextEpisode, raised }: {
  mediaId: string;
  /** MKV chapters fallback while the server has no segment for this title. */
  filePath?: string;
  currentTime: number;
  /** The next-episode card is up: the buttons ride above it. */
  raised?: boolean;
  onSkip: (toSec: number) => void;
  onNextEpisode?: () => void;
}) {
  const { t } = useTranslation();
  const { data } = useMediaSegments(mediaId, [...SKIPPABLE_SEGMENTS]);
  // The server's segments first; the file's own chapters while it has none.
  const hasSegments = (data?.segments?.length ?? 0) > 0;
  const chapterCandidates = useChapterSkippables(filePath, !hasSegments);
  const resolved = activeSkippable(hasSegments ? skippableCandidates(data?.segments) : chapterCandidates, currentTime);
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
  // Takes the remote on arrival, as on the Android television: OK skips.
  return (
    <FocusRoot className={`skip-segment ${raised ? "raised" : ""}`}>
      <Pressable className="skip-btn" focusKey="skip:skip" autoFocus onPress={() => onSkip(resolved.endSec)}>{t(SKIP_LABEL_KEY[resolved.type])}</Pressable>
      {resolved.type === "end_credits" && onNextEpisode ? (
        <Pressable className="skip-btn" focusKey="skip:next" onPress={onNextEpisode}>{t("player.nextEpisodeOverlay.nextEpisode")}</Pressable>
      ) : null}
    </FocusRoot>
  );
}
