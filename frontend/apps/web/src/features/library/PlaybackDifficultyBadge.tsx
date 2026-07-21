import { useTranslation } from "react-i18next";
import { Badge } from "@/components/ui/Badge";
import type { Difficulty, PlaybackDifficulty } from "./playbackDifficulty";

const DIFFICULTY_VARIANT: Record<Difficulty, "success" | "info" | "warning" | "error"> = {
  easy: "success",
  moderate: "info",
  demanding: "warning",
  hard: "error",
};

// One badge, rendered identically wherever a difficulty is known - the media
// page (which asks about one title) and the episode rows (which ask about a
// whole season in one request). Presentational on purpose: the caller decides
// HOW the verdict was fetched, this only decides how it reads.
//
// The label is a plain word and the tooltip is a plain sentence. The server's
// reason codes (hdr_tonemapping_needed, container_incompatible...) are
// diagnostics; they have no business in front of someone picking something to
// watch.
export function PlaybackDifficultyBadge({
  difficulty,
  className = "",
}: {
  difficulty: PlaybackDifficulty;
  className?: string;
}) {
  const { t } = useTranslation();
  return (
    <Badge
      variant={DIFFICULTY_VARIANT[difficulty.difficulty]}
      className={`cursor-help ${className}`}
      title={t(`media.playback.hint.${difficulty.difficulty}`)}
      data-testid="playback-difficulty"
    >
      {t(`media.playback.level.${difficulty.difficulty}`)}
    </Badge>
  );
}
