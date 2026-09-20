import { useState } from "react";
import { View, Text, Pressable, StyleSheet } from "react-native";
import { Image } from "expo-image";
import { Ionicons } from "@expo/vector-icons";
import { useTranslation } from "react-i18next";
import { useNotificationServiceGetUnreadCount, useUserServiceGetCurrentUser, useUserServiceListProfiles } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { useServerUrlNullable } from "../hooks/useServerUrl";
import { useAuthHeaders } from "../hooks/useAuthenticatedImage";

// The header the owner asked for (2026-09-19): the search field, the bell and
// the viewer's own face, top right, on every screen of the shell - the three
// things a television is asked for constantly and that were buried in the
// left rail (search and notifications) or three presses deep inside Settings
// (the profile).
//
// Each one OPENS ITS PAGE - no dropdown. A menu that unfolds over the content
// is a phone gesture; on a remote it is one more thing to dismiss, and the
// pages already exist. Every destination carries its own way back to the home
// screen (the rail, always one LEFT press away, plus the remote's BACK).
export interface TopHeaderProps {
  onSearch: () => void;
  onNotifications: () => void;
  onProfile: () => void;
  // Present on the home only: the home's own "Customize" lives in the header
  // line rather than in a row of its own, which gave the widget below it the
  // room the first poster row needed to stay whole (owner, 2026-09-20).
  onCustomize?: () => void;
}

export function TopHeader({ onSearch, onNotifications, onProfile, onCustomize }: TopHeaderProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const [focused, setFocused] = useState<string | null>(null);
  const serverUrl = useServerUrlNullable();
  const accessToken = useAuthStore((s) => s.accessToken);
  const activeProfileId = useAuthStore((s) => s.profileId);
  const authHeaders = useAuthHeaders();
  // A failed count must never blank the header: no badge is the honest
  // degradation (same rule the rail's bell had).
  const { data: unread, isError: unreadFailed } = useNotificationServiceGetUnreadCount();
  const unreadCount = unreadFailed ? 0 : (unread?.count ?? 0);
  const { data: user } = useUserServiceGetCurrentUser();
  const { data: profilesResp } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const profiles = profilesResp?.profiles ?? [];
  const activeProfile = profiles.find((p: { id?: string }) => p.id === activeProfileId) ?? profiles.find((p: { isDefault?: boolean }) => p.isDefault) ?? profiles[0];
  const initial = (activeProfile?.name ?? user?.username ?? "?").slice(0, 1).toUpperCase();
  const avatarUri = activeProfile?.avatarPath && activeProfile.id && serverUrl && accessToken
    ? `${serverUrl}/api/v1/profiles/${activeProfile.id}/avatar`
    : null;

  const ring = (key: string) => ({
    borderColor: focused === key ? theme.colors.accent : "transparent",
    borderWidth: 3,
    borderRadius: theme.radii.md,
  });

  return (
    <View style={styles.header}>
      {/* Two flexing sides so the field sits in the middle of the screen and
          not wherever the icons leave room (owner, 2026-09-19). */}
      <View style={styles.side} />
      {/* The search field is a button that opens the search screen: a live
          text field up here would hold the remote's focus on every screen and
          swallow the D-pad. It reads as a field because that is what the
          viewer is looking for. */}
      <Pressable
        onPress={onSearch}
        onFocus={() => setFocused("search")}
        onBlur={() => setFocused(null)}
        accessible={true}
        accessibilityRole="search"
        accessibilityLabel={t("common.nav.search")}
        style={[styles.searchField, { backgroundColor: theme.colors.surface }, ring("search")]}
      >
        <Ionicons name="search" size={26} color={focused === "search" ? theme.colors.accent : theme.colors.textMuted} />
        <Text style={[styles.searchText, { color: theme.colors.textMuted }]} numberOfLines={1}>
          {t("common.nav.search")}
        </Text>
      </Pressable>

      <View style={[styles.side, styles.sideRight]}>
      {/* Customize stays at the right edge with the other controls (owner,
          2026-09-20); on the home the field gives up the width it needs. */}
      {onCustomize && (
        <Pressable
          onPress={onCustomize}
          onFocus={() => setFocused("customize")}
          onBlur={() => setFocused(null)}
          accessible={true}
          accessibilityRole="button"
          accessibilityLabel={t("common.feeds.customize")}
          style={[styles.customizeButton, ring("customize")]}
        >
          <Ionicons name="options-outline" size={22} color={focused === "customize" ? theme.colors.accent : theme.colors.textMuted} />
          <Text style={[styles.customizeText, { color: focused === "customize" ? theme.colors.text : theme.colors.textMuted }]} numberOfLines={1}>
            {t("media.home.customize", "Customize")}
          </Text>
        </Pressable>
      )}
      <Pressable
        onPress={onNotifications}
        onFocus={() => setFocused("bell")}
        onBlur={() => setFocused(null)}
        accessible={true}
        accessibilityRole="button"
        accessibilityLabel={t("common.notifications.title")}
        style={[styles.iconButton, ring("bell")]}
      >
        <View>
          <Ionicons name="notifications" size={26} color={focused === "bell" ? theme.colors.accent : theme.colors.textMuted} />
          {unreadCount > 0 && (
            <View style={[styles.badge, { backgroundColor: theme.colors.accent, borderColor: theme.colors.bg }]}>
              <Text style={[styles.badgeText, { color: theme.colors.onAccent }]}>
                {unreadCount > 99 ? "99+" : String(unreadCount)}
              </Text>
            </View>
          )}
        </View>
      </Pressable>

      <Pressable
        onPress={onProfile}
        onFocus={() => setFocused("profile")}
        onBlur={() => setFocused(null)}
        accessible={true}
        accessibilityRole="button"
        accessibilityLabel={t("mobile.settings.switchProfile")}
        style={[styles.iconButton, ring("profile")]}
      >
        <View style={[styles.avatar, { backgroundColor: theme.colors.accent }]}>
          {avatarUri ? (
            <Image source={{ uri: avatarUri, headers: authHeaders }} style={styles.avatarImage} contentFit="cover" />
          ) : (
            <Text style={[styles.avatarInitial, { color: theme.colors.onAccent }]}>{initial}</Text>
          )}
        </View>
      </Pressable>
      </View>
    </View>
  );
}

const styles = StyleSheet.create({
  header: {
    flexDirection: "row",
    alignItems: "center",
    gap: 10,
    paddingHorizontal: 24,
    paddingTop: 16,
    paddingBottom: 8,
  },
  // Each side is at least as wide as its content and at least as wide as the
  // icon side, so the field is centred wherever the sides are even and a
  // side that carries more (the home's Customize) takes its room from the
  // FIELD instead of overflowing onto it (owner, 2026-09-20). Same when the
  // row is narrow - rail unfolded, a denser panel: the field shrinks.
  side: { flexGrow: 1, flexShrink: 0, flexBasis: "auto", minWidth: 120, flexDirection: "row", alignItems: "center", gap: 10 },
  sideRight: { justifyContent: "flex-end" },
  // Wide and tall on purpose (owner, 2026-09-19): it is the header's primary
  // control, it has to read as a field from three metres, and it is the
  // anchor a results panel will drop from.
  searchField: {
    flexDirection: "row",
    alignItems: "center",
    gap: 11,
    width: 760,
    flexShrink: 1,
    minWidth: 0,
    paddingVertical: 11,
    paddingHorizontal: 22,
  },
  searchText: { fontSize: 20 },
  iconButton: { padding: 8 },
  customizeButton: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 8, paddingHorizontal: 12 },
  customizeText: { fontSize: 17 },
  avatar: { width: 40, height: 40, borderRadius: 20, alignItems: "center", justifyContent: "center", overflow: "hidden" },
  avatarImage: { width: "100%", height: "100%" },
  avatarInitial: { fontSize: 17, fontWeight: "700" },
  badge: {
    position: "absolute",
    top: -6,
    right: -10,
    minWidth: 22,
    height: 22,
    borderRadius: 11,
    borderWidth: 2,
    alignItems: "center",
    justifyContent: "center",
    paddingHorizontal: 4,
  },
  badgeText: { fontSize: 12, fontWeight: "700" },
});
