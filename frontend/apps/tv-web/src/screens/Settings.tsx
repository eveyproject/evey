import { useTranslation } from "react-i18next";
import { useServerListStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button } from "../components/ui";
import { replace } from "../app/router";
import { signOut } from "../lib/session";
import i18n, { supportedLanguages, saveLocale } from "../i18n";

export function SettingsScreen() {
  const { t } = useTranslation();
  const server = useServerListStore((s) => s.getActiveServer());

  return (
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">{t("common.nav.settings", "Settings")}</h1>
      <p className="subtitle">{server?.label || server?.serverName || server?.url}</p>
      <div className="actions">
        {supportedLanguages.map((lng) => (
          <Button key={lng} variant={i18n.language === lng ? "primary" : "default"} onPress={() => { saveLocale(lng); void i18n.changeLanguage(lng); }}>
            {lng.toUpperCase()}
          </Button>
        ))}
      </div>
      <div className="actions">
        <Button autoFocus onPress={() => replace({ name: "profiles" })}>{t("common.nav.switchProfile", "Switch profile")}</Button>
        <Button onPress={() => { void signOut().then(() => replace({ name: "login" })); }}>{t("common.nav.signOut", "Sign out")}</Button>
        <Button variant="ghost" onPress={() => { void signOut().then(() => replace({ name: "server" })); }}>{t("mobile.settings.changeServer", "Change server")}</Button>
      </div>
    </FocusRoot>
  );
}
