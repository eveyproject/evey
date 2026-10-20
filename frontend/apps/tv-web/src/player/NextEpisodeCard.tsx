import { useTranslation } from "react-i18next";
import type { useNextEpisode } from "@evey/client";
import { episodeLabel } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Pressable } from "../components/ui";

type NextEpisode = ReturnType<typeof useNextEpisode>;

// "Up next" near the end of an episode, then a countdown once it ended. The
// remote lands on Play now; Cancel lets the credits run.
export function NextEpisodeCard({ nextEp }: { nextEp: NextEpisode }) {
  const { t } = useTranslation();
  if (!nextEp.nextEpisode) return null;
  return (
    <FocusRoot className="nextep-card">
      <div className="nextep-heading">
        {nextEp.stillWatching
          ? t("player.nextEpisodeOverlay.stillWatching", "Still watching?")
          : nextEp.ended
            ? t("player.nextEpisodeOverlay.nextIn", { seconds: Math.max(nextEp.countdown, 0) })
            : t("player.nextEpisodeOverlay.upNext", "Up next")}
      </div>
      <div className="nextep-title">
        {nextEp.nextEpisode.indexNumber != null
          ? episodeLabel({
              seasonNumber: nextEp.nextEpisode.seasonIndexNumber,
              episodeNumber: nextEp.nextEpisode.indexNumber,
              title: nextEp.nextEpisode.title,
            }) || t("player.nextEpisodeOverlay.episode", { number: nextEp.nextEpisode.indexNumber })
          : [t("player.nextEpisodeOverlay.nextEpisode", "Next episode"), nextEp.nextEpisode.title].filter(Boolean).join(" · ")}
      </div>
      <div className="nextep-actions">
        <Pressable className="tv-card nextep-btn primary" focusKey="nextep:play" autoFocus onPress={nextEp.playNow}>{t("player.nextEpisodeOverlay.playNow", "Play now")}</Pressable>
        <Pressable className="tv-card nextep-btn" focusKey="nextep:cancel" onPress={nextEp.cancel}>{t("common.cancel", "Cancel")}</Pressable>
      </div>
    </FocusRoot>
  );
}
