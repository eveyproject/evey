import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceUpdateProfile, getUserServiceListProfilesQueryKey } from "@evey/api";
import { autoplayNextEpisode, autoplayNextPreferences } from "@evey/store";
import { Switch } from "@/components/ui/Switch";
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

  const handleToggle = async () => {
    if (!activeProfile?.id) return;
    try {
      await updateProfile.mutateAsync({
        id: activeProfile.id,
        data: { preferences: autoplayNextPreferences(activeProfile.preferences, !enabled) },
      });
      if (userId) void qc.invalidateQueries({ queryKey: getUserServiceListProfilesQueryKey(userId) });
    } catch (err) { console.warn("autoplay next update failed", err); }
  };

  return (
    <div className="flex items-center justify-between gap-4">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">{t("auth.settings.playback.autoplayNextTitle")}</h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">{t("auth.settings.playback.autoplayNextDesc")}</p>
      </div>
      <Switch
        checked={enabled}
        onChange={handleToggle}
        label={t("auth.settings.playback.autoplayNextTitle")}
        disabled={!activeProfile}
        data-testid="autoplay-next-switch"
      />
    </div>
  );
}
