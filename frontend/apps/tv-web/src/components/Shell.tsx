import { useCallback, useState, type ReactNode } from "react";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount } from "@evey/api";
import { useActiveProfile } from "@evey/client";
import { useDemoStore } from "@evey/store";
import { FocusRoot, useBackHandler } from "../tv/focus";
import { navigate, replace, type Route } from "../app/router";
import { HOME_CUSTOMIZE_EVENT } from "./StackHeader";
import { avatarSrc } from "../lib/media";
import { Pressable } from "./ui";
import { Icon } from "./icons";
import type { IoniconName } from "./ionicons";
import { EveyMark, EveyWordmark } from "./EveyLogo";
import { SearchPanel } from "./SearchPanel";

// The screens that carry the rail and the header, as the Android television's
// (tabs) group does. Every other screen is a deep one, with the home button.
export const TAB_ROUTES = new Set<Route["name"]>(["home", "libraries", "music", "favorites", "requests", "settings", "search"]);

const NAV_ITEMS: { name: Route["name"]; labelKey: string; icon: IoniconName }[] = [
  { name: "home", labelKey: "common.nav.home", icon: "home" },
  { name: "libraries", labelKey: "common.nav.libraries", icon: "library" },
  { name: "music", labelKey: "common.nav.music", icon: "musical-notes" },
  { name: "favorites", labelKey: "common.nav.favorites", icon: "heart" },
  { name: "requests", labelKey: "common.nav.requests", icon: "mail-unread" },
  { name: "settings", labelKey: "common.nav.settings", icon: "settings" },
];

function Rail({ current }: { current: Route["name"] }) {
  const { t } = useTranslation();
  const [expanded, setExpanded] = useState(false);
  const open = useCallback(() => setExpanded(true), []);
  const close = useCallback(() => setExpanded(false), []);
  return (
    <FocusRoot focusKey="rail" className={`rail ${expanded ? "expanded" : ""}`}>
      <div className="rail-brand">{expanded ? <EveyWordmark height={20} /> : <EveyMark height={24} />}</div>
      {NAV_ITEMS.map((item) => (
        <Pressable
          key={item.name}
          focusKey={`rail:${item.name}`}
          className={`rail-item ${current === item.name ? "active" : ""}`}
          label={t(item.labelKey)}
          onPress={() => navigate({ name: item.name } as Route)}
          onFocus={open}
          onBlur={close}
        >
          <Icon name={item.icon} size={28} />
          {expanded ? <span className="rail-label">{t(item.labelKey)}</span> : null}
        </Pressable>
      ))}
    </FocusRoot>
  );
}

function TopHeader({ current, onSearch }: { current: Route["name"]; onSearch: () => void }) {
  const { t } = useTranslation();
  const { data: unread, isError } = useNotificationServiceGetUnreadCount(undefined, { query: { refetchInterval: 60_000 } });
  const count = isError ? 0 : (unread?.count ?? 0);
  const { profile } = useActiveProfile();
  const initial = (profile?.name ?? "?").slice(0, 1).toUpperCase();
  // The demo notice where the tab screens have room for it: the header's empty
  // left side (a pill over the header would cover the search field).
  const demo = useDemoStore((s) => s.demoMode);
  return (
    <FocusRoot focusKey="header" className="top-header">
      <div className="header-side">
        {demo ? <span className="demo-tag" title={t("common.demo.banner")} data-testid="demo-tag">{t("common.demo.tag")}</span> : null}
      </div>
      <Pressable className="search-field" label={t("common.nav.search")} onPress={onSearch}>
        <Icon name="search" size={26} />
        <span className="search-field-text">{t("common.nav.search")}</span>
      </Pressable>
      <div className="header-side right">
        {current === "home" && !demo ? (
          <Pressable className="customize-btn" label={t("common.feeds.customize")} onPress={() => window.dispatchEvent(new Event(HOME_CUSTOMIZE_EVENT))}>
            <Icon name="options-outline" size={22} />
            <span>{t("media.home.customize", "Customize")}</span>
          </Pressable>
        ) : null}
        <Pressable className="icon-btn" label={t("common.notifications.title")} onPress={() => navigate({ name: "notifications" })}>
          <span className="bell">
            <Icon name="notifications" size={26} />
            {count > 0 ? <span className="badge">{count > 99 ? "99+" : count}</span> : null}
          </span>
        </Pressable>
        <Pressable className="icon-btn" label={t("mobile.settings.switchProfile")} onPress={() => navigate({ name: "profiles", pick: true })}>
          <span className="avatar">
            {profile?.avatarPath && profile.id ? <img src={avatarSrc(profile.id)} alt="" /> : initial}
          </span>
        </Pressable>
      </div>
    </FocusRoot>
  );
}

function SearchOverlay({ onClose }: { onClose: () => void }) {
  useBackHandler(onClose);
  return (
    <div className="search-overlay">
      <div className="search-scrim" onClick={onClose} />
      <FocusRoot autoFocus boundary className="search-overlay-panel">
        <SearchPanel onNavigate={onClose} />
      </FocusRoot>
    </div>
  );
}

/** The tab screens' frame: the rail on the left, the header over the screen. */
export function TabShell({ current, children }: { current: Route["name"]; children: ReactNode }) {
  const [searchOpen, setSearchOpen] = useState(false);
  const openSearch = useCallback(() => setSearchOpen(true), []);
  const closeSearch = useCallback(() => setSearchOpen(false), []);
  return (
    <div className="shell">
      <Rail current={current} />
      <div className="shell-content">
        <TopHeader current={current} onSearch={openSearch} />
        <div className="shell-body">{children}</div>
      </div>
      {searchOpen ? <SearchOverlay onClose={closeSearch} /> : null}
    </div>
  );
}

/** The way back to the home from a deep screen, floating in the corner. */
export function HomeShortcut({ raised }: { raised?: boolean }) {
  const { t } = useTranslation();
  return (
    <FocusRoot focusKey="home-shortcut" className={`home-shortcut ${raised ? "raised" : ""}`}>
      <Pressable className="home-shortcut-btn" label={t("common.nav.home")} onPress={() => replace({ name: "home" })}>
        <Icon name="home" size={28} />
      </Pressable>
    </FocusRoot>
  );
}

