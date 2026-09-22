import { useCallback, useEffect, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useFeedServiceResetProfileFeedConfig,
  useFeedServiceUpdateProfileFeedConfig,
} from "@evey/api";
import { buildFeedRows, moveRow, toggleRow, type FeedConfigEntry, type FeedRow } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button } from "./ui";

const FEED_LABELS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  continue_watching: "common.feeds.labels.resume_playback",
  next_episode: "media.home.upNext",
  collections: "media.home.collections",
  recommended: "media.home.recommendedForYou",
  most_popular: "media.home.mostPopular",
};

interface Props {
  configs: FeedConfigEntry[];
  profileId: string;
  libraryNames: Record<string, string>;
  pluginNames: Record<string, string>;
  onClose: () => void;
}

/**
 * The home's row order and visibility, the same panel the television and the
 * phone show: OK on a row hides or shows it, the arrows move it, and every
 * change is written to the profile straight away.
 */
export function FeedCustomizePanel({ configs, profileId, libraryNames, pluginNames, onClose }: Props) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const updateFeed = useFeedServiceUpdateProfileFeedConfig();
  const resetFeed = useFeedServiceResetProfileFeedConfig();
  const [rows, setRows] = useState<FeedRow[]>(() => buildFeedRows(configs, libraryNames, pluginNames));
  const [error, setError] = useState(false);
  // A remote has no "are you sure" dialog worth building: the first OK asks,
  // the second, on the now relabelled button, commits.
  const [confirmingReset, setConfirmingReset] = useState(false);

  useEffect(() => {
    setRows(buildFeedRows(configs, libraryNames, pluginNames));
  }, [configs, libraryNames, pluginNames]);

  const persist = useCallback(async (next: FeedRow[]) => {
    let failed = 0;
    for (const row of next) {
      if (!row.feedType) continue;
      try {
        await updateFeed.mutateAsync({ feedType: row.feedType, data: { profileId, enabled: row.enabled, displayOrder: row.displayOrder } });
      } catch {
        failed++;
      }
    }
    await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
    // Saying nothing on a partial failure would leave the viewer believing an
    // order the server does not have.
    setError(failed > 0);
  }, [updateFeed, profileId, qc]);

  const apply = (next: FeedRow[]) => { setRows(next); void persist(next); };

  const handleReset = async () => {
    if (!confirmingReset) { setConfirmingReset(true); return; }
    setConfirmingReset(false);
    let failed = 0;
    for (const row of rows) {
      if (!row.feedType) continue;
      try { await resetFeed.mutateAsync({ feedType: row.feedType }); } catch { failed++; }
    }
    await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
    setError(failed > 0);
  };

  const label = (feedType: string): string => {
    if (feedType.startsWith("library:")) {
      const id = feedType.replace("library:", "");
      return libraryNames[id] || id;
    }
    if (feedType.startsWith("plugin:")) {
      const id = feedType.replace("plugin:", "");
      const raw = pluginNames[id] || id;
      // A plugin manifest carries no display name, so the raw one would sit
      // in lower case next to properly formatted shelf labels.
      return raw.charAt(0).toUpperCase() + raw.slice(1);
    }
    return t(FEED_LABELS[feedType] ?? feedType, feedType);
  };

  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus className="modal-sheet">
        <h2 className="row-title">{t("media.home.customizeFeed", "Customize Feed")}</h2>
        <p className="muted">{t("media.home.customizeHintTv", "Press OK on a row to show or hide it")}</p>
        {error ? <p className="error">{t("media.home.customizeSaveFailed", "Failed to save some changes")}</p> : null}
        <div className="feed-rows">
          {rows.map((row, i) => (
            <div className="feed-row" key={row.feedType}>
              <Button autoFocus={i === 0} variant={row.enabled ? "primary" : "default"} onPress={() => apply(toggleRow(rows, i))}>
                {row.enabled ? "◉" : "○"} {label(row.feedType)}
              </Button>
              <Button disabled={i === 0} onPress={() => apply(moveRow(rows, i, -1))}>↑</Button>
              <Button disabled={i === rows.length - 1} onPress={() => apply(moveRow(rows, i, 1))}>↓</Button>
            </div>
          ))}
        </div>
        <div className="actions">
          <Button onPress={handleReset}>{t(confirmingReset ? "common.feeds.resetConfirm" : "common.feeds.resetDefaults")}</Button>
          <Button variant="primary" onPress={onClose}>{t("common.close", "Close")}</Button>
        </div>
      </FocusRoot>
    </div>
  );
}
