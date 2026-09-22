import { useTranslation } from "react-i18next";
import { useLibraryServiceListLibraries, useNotificationServiceGetUnreadCount } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Button } from "./ui";
import { navigate, type Route } from "../app/router";

/**
 * The navigation the Android television keeps in its sidebar, on every
 * screen: home, libraries, music, favourites, search, notifications with
 * their unread count, settings. A television has no browser chrome, so a
 * screen without it is a screen the remote can only leave by going back.
 */
export function NavBar({ current }: { current: Route["name"] }) {
  const { t } = useTranslation();
  const { data: libs } = useLibraryServiceListLibraries();
  const hasMusic = (libs?.libraries ?? []).some((l) => l.type === "LIBRARY_TYPE_MUSIC");
  const { data: unread } = useNotificationServiceGetUnreadCount(undefined, { query: { refetchInterval: 60_000 } });
  const count = unread?.count ?? 0;
  const item = (name: Route["name"], label: string, go: () => void) => (
    <Button variant={current === name ? "primary" : "default"} onPress={go}>{label}</Button>
  );
  return (
    <FocusRoot focusKey="nav" className="navbar">
      {item("home", t("common.nav.home"), () => navigate({ name: "home" }))}
      {item("libraries", t("media.library.libraries"), () => navigate({ name: "libraries" }))}
      {hasMusic ? item("music", t("common.nav.music"), () => navigate({ name: "music" })) : null}
      {item("favorites", t("media.favorites.title"), () => navigate({ name: "favorites" }))}
      {item("requests", t("common.nav.requests"), () => navigate({ name: "requests" }))}
      {item("search", t("common.nav.search"), () => navigate({ name: "search" }))}
      {item("notifications", `${t("common.notifications.title")}${count > 0 ? ` (${count})` : ""}`, () => navigate({ name: "notifications" }))}
      {item("settings", t("common.nav.settings"), () => navigate({ name: "settings" }))}
    </FocusRoot>
  );
}
