import { useTranslation } from "react-i18next";
import { usePluginList } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button } from "../components/ui";
import { navigate } from "../app/router";
import { PluginSlot } from "../plugins/PluginSlot";

/** One plugin's own settings module, as the television hosts it. */
export function PluginSettingsScreen({ pluginId }: { pluginId: string }) {
  const { t } = useTranslation();
  const { data } = usePluginList();
  const plugin = (data?.plugins ?? []).find((p) => p.id === pluginId);
  return (
    <div className="screen">
      <h1 className="title">{plugin?.name ?? pluginId}</h1>
      <div className="plugin-row">
        <PluginSlot name="settings.user.appearance" pluginId={pluginId} />
      </div>
      <FocusRoot autoFocus className="actions">
        <Button onPress={() => navigate({ name: "settings" })}>{t("common.back")}</Button>
      </FocusRoot>
    </div>
  );
}
