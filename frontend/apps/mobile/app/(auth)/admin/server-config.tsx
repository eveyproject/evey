import { useCallback, useRef, useState } from "react";
import { RefreshControl, ScrollView, StyleSheet, View } from "react-native";
import { Stack } from "expo-router";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  eveyFetch,
  apiErrorMessage,
  getServerConfigServiceListServerConfigEntriesQueryKey,
  configDefaultsKey,
} from "@evey/api";
import { toast } from "@evey/store";
import { SERVER_SETTINGS_KEY, UPDATE_STATUS_QUERY_KEY } from "@evey/client";
import { useTheme } from "../../../src/providers/ThemeProvider";
import { SectionTabs } from "../../../src/components/admin/settings/SectionTabs";
import {
  SERVER_CONFIG_SECTIONS,
  ServerConfigSection,
  type ConfigEntryOps,
  type ServerConfigSectionId,
} from "../../../src/components/admin/server-config/ServerConfigSections";
import { SSO_QUERY_KEY } from "../../../src/components/admin/server-config/SsoSettingsCard";
import type { ConfigEntry } from "../../../src/components/admin/server-config/types";

// The generated client's key and response shape, so the Transcoding screen
// (which reads the same entries through the generated hook) and this one
// share one cache: an edit here refreshes it there, and back.
const SERVER_CONFIG_ENTRIES_KEY = getServerConfigServiceListServerConfigEntriesQueryKey();
type EntriesResponse = { entries?: { key?: string; value?: string }[] };

// useConfigEntryOps owns the raw config-entry reads and writes every card of
// the screen shares (PUT/DELETE /api/v1/admin/server-config/:key).
function useConfigEntryOps(): ConfigEntryOps & { refetch: () => Promise<unknown> } {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: entries, refetch } = useQuery({
    queryKey: SERVER_CONFIG_ENTRIES_KEY,
    queryFn: ({ signal }) => eveyFetch<EntriesResponse>({ url: "/api/v1/admin/server-config", method: "GET", signal }),
    select: (d): ConfigEntry[] => (d.entries ?? []).map((e) => ({ key: e.key ?? "", value: e.value ?? "" })),
  });

  const putEntry = ({ key, value }: { key: string; value: string }) =>
    eveyFetch({ url: `/api/v1/admin/server-config/${key}`, method: "PUT", data: { value } });
  const delEntry = (key: string) => eveyFetch({ url: `/api/v1/admin/server-config/${key}`, method: "DELETE" });

  const save = useMutation({
    mutationFn: putEntry,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SERVER_CONFIG_ENTRIES_KEY });
      toast.success(t("admin.serverConfig.configUpdated", "Config saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });
  // The SMTP and pace Saves write six keys each and toast the outcome once:
  // these writes carry no toast of their own, nor the error net's.
  const quietSave = useMutation({
    mutationFn: putEntry,
    onSuccess: () => qc.invalidateQueries({ queryKey: SERVER_CONFIG_ENTRIES_KEY }),
    meta: { silentError: true },
  });
  const del = useMutation({
    mutationFn: delEntry,
    onSuccess: async () => {
      await qc.invalidateQueries({ queryKey: SERVER_CONFIG_ENTRIES_KEY });
      toast.success(t("admin.serverConfig.configUpdated", "Config saved"));
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });
  const quietDel = useMutation({
    mutationFn: delEntry,
    onSuccess: () => qc.invalidateQueries({ queryKey: SERVER_CONFIG_ENTRIES_KEY }),
    meta: { silentError: true },
  });

  return {
    entries,
    refetch,
    getEntryValue: (key) => entries?.find((e) => e.key === key)?.value ?? "",
    saveEntry: save.mutate,
    saveEntryAsync: save.mutateAsync,
    deleteEntry: (key) => del.mutate(key),
    deleteEntryAsync: del.mutateAsync,
    quietSaveEntry: quietSave.mutateAsync,
    quietDeleteEntry: quietDel.mutateAsync,
    savePending: save.isPending,
    deletePending: del.isPending,
    quietPending: quietSave.isPending || quietDel.isPending,
  };
}

// AdminServerConfigScreen splits the server settings into the web's eight
// sections behind a row of tabs that stays on top while the page scrolls.
// Tabs rather than a list of sub-screens: every section stays mounted (only
// hidden), so an edit typed in one section survives a look at another before
// its Save, and switching is one tap with the open tab always in view.
export default function AdminServerConfigScreen() {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const ops = useConfigEntryOps();
  const [section, setSection] = useState<ServerConfigSectionId>("general");
  const scroller = useRef<ScrollView>(null);

  // A new section starts at its top, not at the depth the last one was read
  // to; the tab row stays pinned above it.
  function open(id: ServerConfigSectionId) {
    setSection(id);
    scroller.current?.scrollTo?.({ y: 0, animated: false });
  }
  const [refreshing, setRefreshing] = useState(false);
  const { refetch } = ops;

  const onRefresh = useCallback(async () => {
    setRefreshing(true);
    // Everything a section reads: the settings, the raw entries (every card
    // built on them), the SSO card, the update status and the locale defaults.
    await Promise.all([
      qc.invalidateQueries({ queryKey: SERVER_SETTINGS_KEY }),
      refetch(),
      qc.invalidateQueries({ queryKey: SSO_QUERY_KEY }),
      qc.invalidateQueries({ queryKey: UPDATE_STATUS_QUERY_KEY }),
      qc.invalidateQueries({ queryKey: configDefaultsKey }),
    ]);
    setRefreshing(false);
  }, [qc, refetch]);

  return (
    <View style={{ flex: 1, backgroundColor: theme.colors.bg }}>
      <Stack.Screen
        options={{
          headerShown: true,
          title: t("admin.serverConfig.title", "Server Config"),
          headerStyle: { backgroundColor: theme.colors.surface },
          headerTintColor: theme.colors.text,
        }}
      />
      <ScrollView
        ref={scroller}
        keyboardShouldPersistTaps="handled"
        // iOS: the scroll insets follow the keyboard and a focused field is
        // scrolled above it (Android resizes the window itself).
        automaticallyAdjustKeyboardInsets
        stickyHeaderIndices={[0]}
        style={{ flex: 1 }}
        contentContainerStyle={styles.content}
        refreshControl={<RefreshControl refreshing={refreshing} onRefresh={onRefresh} tintColor={theme.colors.accent} />}
      >
        <SectionTabs
          label={t("admin.settingsLayout.sectionsLabel")}
          tabs={SERVER_CONFIG_SECTIONS.map((id) => ({ id, label: t(`admin.settingsLayout.sections.${id}.title`) }))}
          value={section}
          onChange={(id) => open(id as ServerConfigSectionId)}
          testID="server-config-sections"
        />
        <View style={styles.body}>
          {SERVER_CONFIG_SECTIONS.map((id) => (
            <View key={id} style={id === section ? undefined : styles.hidden} accessibilityElementsHidden={id !== section} importantForAccessibility={id === section ? "auto" : "no-hide-descendants"}>
              <ServerConfigSection id={id} ops={ops} />
            </View>
          ))}
        </View>
      </ScrollView>
    </View>
  );
}

const styles = StyleSheet.create({
  content: { paddingBottom: 40 },
  body: { padding: 16 },
  hidden: { display: "none" },
});
