import { useTranslation } from "react-i18next";
import { resolveSlotEntries, usePluginList } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Pressable } from "../components/ui";
import { goBack } from "../app/router";
import { PluginSlot } from "../plugins/PluginSlot";

/** One plugin's own settings module, as the Android television hosts it. */
export function PluginSettingsScreen({ pluginId }: { pluginId: string }) {
  const { t } = useTranslation();
  const { data } = usePluginList();
  const entry = resolveSlotEntries(data?.plugins, "settings.user.appearance", { pluginId })[0];
  const name = (data?.plugins ?? []).find((p) => p.id === pluginId)?.name;
  if (data && !entry) {
    return (
      <FocusRoot autoFocus className="center-fill plugin-missing">
        <p className="plugin-missing-text">{t("admin.plugins.notFound", "This plugin is not installed, enabled, or has no admin page.")}</p>
        <Pressable className="tv-card state-btn" autoFocus onPress={goBack}>{t("common.back", "Back")}</Pressable>
      </FocusRoot>
    );
  }
  return (
    <div className="screen set-page">
      <h1 className="set-title">{name ?? t("auth.settings.tabs.plugins")}</h1>
      <PluginSlot name="settings.user.appearance" pluginId={pluginId} />
    </div>
  );
}
