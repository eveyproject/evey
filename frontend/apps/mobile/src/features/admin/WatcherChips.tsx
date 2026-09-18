import { useMemo } from "react";
import { ScrollView, Pressable, Text } from "react-native";
import { useTranslation } from "react-i18next";
import { useUserServiceListUsers, useUserServiceListAllProfiles } from "@evey/api";
import { useTheme } from "../../providers/ThemeProvider";

// A watcher is either a whole account or one profile of it - the two ids the
// activity API takes.
export type Watcher = { userId?: string; profileId?: string };

function sameWatcher(a: Watcher, b: Watcher): boolean {
  return a.userId === b.userId && a.profileId === b.profileId;
}

// WatcherChips answers "whose activity am I looking at?" on a phone: a row of
// chips rather than the web's <select>, because a household has a handful of
// profiles and a picker modal would cost a tap for nothing.
export function WatcherChips({ value, onChange }: { value: Watcher; onChange: (w: Watcher) => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const { data: userData } = useUserServiceListUsers();
  const { data: profileData } = useUserServiceListAllProfiles();

  const chips = useMemo(() => {
    const users = userData?.users ?? [];
    const profiles = profileData?.profiles ?? [];
    const usernames = new Map(users.map((u) => [u.id ?? "", u.username ?? ""]));
    const out: Array<{ key: string; label: string; watcher: Watcher }> = [
      { key: "all", label: t("admin.activity.watcherAll", "Everyone"), watcher: {} },
    ];
    for (const p of profiles) {
      const owner = usernames.get(p.userId ?? "");
      out.push({
        key: p.id ?? "",
        // Two households can both name a profile "Papa"; the owner
        // disambiguates without a second line of text.
        label: owner ? `${p.name} · ${owner}` : (p.name ?? ""),
        watcher: { profileId: p.id },
      });
    }
    return out;
  }, [userData, profileData, t]);

  // One profile on one account: there is nothing to filter between.
  if (chips.length <= 2) return null;

  // flexShrink 0: the list below always wants more height than the screen, and
  // Yoga would otherwise squeeze this row until the chips are clipped.
  return (
    <ScrollView
      horizontal
      showsHorizontalScrollIndicator={false}
      contentContainerStyle={{ paddingHorizontal: 16, paddingVertical: 8, gap: 6 }}
      style={{ flexGrow: 0, flexShrink: 0, backgroundColor: theme.colors.surface }}
    >
      {chips.map((c) => {
        const active = sameWatcher(c.watcher, value);
        return (
          <Pressable
            key={c.key}
            onPress={() => onChange(c.watcher)}
            accessibilityRole="button"
            accessibilityState={{ selected: active }}
            style={{
              paddingHorizontal: 10,
              paddingVertical: 5,
              borderRadius: theme.radii.sm,
              backgroundColor: active ? theme.colors.accent : theme.colors.surfaceRaised,
            }}
          >
            <Text
              style={{
                color: active ? theme.colors.onAccent : theme.colors.textMuted,
                fontSize: 12,
                fontWeight: "600",
              }}
            >
              {c.label}
            </Text>
          </Pressable>
        );
      })}
    </ScrollView>
  );
}
