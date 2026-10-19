import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceUpdateProfile, getUserServiceListProfilesQueryKey } from "@evey/api";
import { autoplayNextEpisode, autoplayNextPreferences } from "@evey/store";
import { Switch } from "@/components/ui/Switch";
import { Select } from "@/components/ui/Select";
import { STILL_WATCHING_STEPS, stillWatchingLimit, stillWatchingPreferences } from "@evey/client";
import { useActiveProfile } from "@/hooks/useActiveProfile";

// Per-profile: whether the next episode starts on its own at the end of the
// current one. The player offers the same switch in its settings menu; both
// write the same profile preference so the choice follows the viewer across
// devices. Operates on the ACTIVE profile, not the default one.
export function AutoplayNextSettings() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { profile: activeProfile, userId } = useActiveProfile();
  const updateProfile = useUserServiceUpdateProfile();

  const enabled = autoplayNextEpisode(activeProfile?.preferences);
  const stillWatchingAfter = stillWatchingLimit(activeProfile?.preferences);

  // Both controls merge into the cached preferences and the server replaces
  // the whole map: a second write sent before the first one's refetch landed
  // dropped the first. One write at a time, released once the profile is
  // fresh again.
  const [writing, setWriting] = useState(false);
  const writePreferences = async (preferences: Record<string, string>, what: string) => {
    if (!activeProfile?.id) return;
    setWriting(true);
    try {
      await updateProfile.mutateAsync({ id: activeProfile.id, data: { preferences } });
      if (userId) await qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
    } catch (err) {
      console.warn(`${what} update failed`, err);
    } finally {
      setWriting(false);
    }
  };

  const handleToggle = () => writePreferences(autoplayNextPreferences(activeProfile?.preferences, !enabled), "autoplay next");
  const handleStillWatching = (value: string) => writePreferences(stillWatchingPreferences(activeProfile?.preferences, Number(value)), "still watching");

  return (
    <div className="flex flex-col gap-4">
    <div className="flex items-center justify-between gap-4">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.autoplayNextTitle")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.playback.autoplayNextDesc")}</p>
      </div>
      <Switch
        checked={enabled}
        onChange={handleToggle}
        label={t("auth.settings.playback.autoplayNextTitle")}
        disabled={!activeProfile || writing}
        data-testid="autoplay-next-switch"
      />
    </div>
    {/* Only meaningful while episodes follow on their own. */}
    {enabled && (
      <div className="flex items-center justify-between gap-4">
        <div>
          <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.stillWatchingTitle")}</h3>
          <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.playback.stillWatchingDesc")}</p>
        </div>
        <Select
          label={t("auth.settings.playback.stillWatchingTitle")}
          options={STILL_WATCHING_STEPS.map((n) => ({
            value: String(n),
            label: n === 0 ? t("auth.settings.playback.stillWatchingNever") : t("auth.settings.playback.stillWatchingAfter", { count: n }),
          }))}
          value={String(stillWatchingAfter)}
          onChange={(v) => void handleStillWatching(v)}
          disabled={!activeProfile || writing}
          data-testid="still-watching-select"
        />
      </div>
    )}
    </div>
  );
}
