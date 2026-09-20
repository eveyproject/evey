import { useCallback, useEffect, useRef, useState } from "react";
import { View, Text, Modal, ScrollView, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Ionicons } from "@expo/vector-icons";
import { useFeedServiceUpdateProfileFeedConfig, useFeedServiceResetProfileFeedConfig } from "@evey/api";
import { toast } from "@evey/store";
import { useTheme } from "../providers/ThemeProvider";
import { FocusableCard } from "./FocusableCard";
import { buildFeedRows, moveRow, toggleRow, type FeedRow } from "../lib/feedCustomize";
import type { FeedConfigEntry } from "../lib/homeSections";
import { FONT_SIZE } from "@evey/theme";

// Reordering and hiding the home rows, from the sofa.
//
// Mobile has had this since day one and the television could only read the
// config somebody else had set. Same server calls, same rules (see
// lib/feedCustomize), different input device - which is the whole design
// problem here: mobile puts three controls on every row and lets you tap the
// one you want. A remote has to travel to each of them, so a ten-row list
// would cost thirty presses to cross.
//
// The first attempt bound LEFT and RIGHT to "move this row". It was cheap to
// navigate and impossible to discover: nothing on screen says a direction key
// does something other than move the focus. So the arrows are real, focusable
// buttons - more presses, but the panel explains itself.

const FEED_LABELS: Record<string, string> = {
  resume_playback: "common.feeds.labels.resume_playback",
  continue_watching: "common.feeds.labels.resume_playback",
  next_episode: "media.home.upNext",
  collections: "media.home.collections",
  recommended: "media.home.recommendedForYou",
  most_popular: "media.home.mostPopular",
};

interface FeedCustomizePanelProps {
  visible: boolean;
  onClose: () => void;
  configs: FeedConfigEntry[];
  profileId: string;
  libraryNames: Record<string, string>;
  pluginNames: Record<string, string>;
}

export function FeedCustomizePanel({
  visible,
  onClose,
  configs,
  profileId,
  libraryNames,
  pluginNames,
}: FeedCustomizePanelProps) {
  const theme = useTheme();
  const { t } = useTranslation();
  const qc = useQueryClient();
  const updateFeed = useFeedServiceUpdateProfileFeedConfig();
  const resetFeed = useFeedServiceResetProfileFeedConfig();
  const [rows, setRows] = useState<FeedRow[]>([]);
  // A remote has no long-press and no "are you sure" dialog worth building
  // for this: the first OK press asks, the second (on the same, now
  // relabelled button) commits. Reopening the panel drops the pending
  // confirmation rather than leaving a scary label behind.
  const [confirmingReset, setConfirmingReset] = useState(false);
  // Two OK presses fired in quick succession (repeat-key or a double click)
  // land before the viewer ever reads "Confirmer ?": lock the button for the
  // time it takes to read the relabelled state, then release it for the
  // real second press.
  const [resetLocked, setResetLocked] = useState(false);
  const resetLockTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  useEffect(() => () => {
    if (resetLockTimer.current) clearTimeout(resetLockTimer.current);
  }, []);

  useEffect(() => {
    if (visible) {
      setRows(buildFeedRows(configs, libraryNames, pluginNames));
      setConfirmingReset(false);
      setResetLocked(false);
    }
  }, [visible, configs, libraryNames, pluginNames]);

  const persist = useCallback(
    async (next: FeedRow[]) => {
      let failed = 0;
      for (const row of next) {
        if (!row.feedType) continue;
        try {
          await updateFeed.mutateAsync({
            feedType: row.feedType,
            data: { profileId, enabled: row.enabled, displayOrder: row.displayOrder },
          });
        } catch {
          failed++;
        }
      }
      await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
      // Saying nothing on a partial failure would leave the viewer believing an
      // order that the server does not have.
      if (failed > 0) toast.error(t("media.home.customizeSaveFailed", "Failed to save some changes"));
    },
    [updateFeed, profileId, qc, t],
  );

  const apply = useCallback(
    (next: FeedRow[]) => {
      setRows(next);
      void persist(next);
    },
    [persist],
  );

  const handleReset = useCallback(async () => {
    if (!confirmingReset) {
      setConfirmingReset(true);
      setResetLocked(true);
      // Owned by a ref so closing the panel mid-window clears it instead of
      // firing setState on an unmounted component.
      resetLockTimer.current = setTimeout(() => setResetLocked(false), 600);
      return;
    }
    setConfirmingReset(false);
    let failed = 0;
    for (const row of rows) {
      if (!row.feedType) continue;
      try {
        await resetFeed.mutateAsync({ feedType: row.feedType });
      } catch {
        failed++;
      }
    }
    // The `configs` -> `rows` effect above picks up the refreshed server
    // state once this invalidation resolves; no need to reset `rows` by hand.
    await qc.invalidateQueries({ queryKey: ["/api/v1/feeds/config"] });
    if (failed > 0) toast.error(t("media.home.customizeSaveFailed", "Failed to save some changes"));
  }, [confirmingReset, rows, resetFeed, qc, t]);

  const label = (feedType: string): string => {
    if (feedType.startsWith("library:")) {
      const id = feedType.replace("library:", "");
      return libraryNames[id] || id;
    }
    if (feedType.startsWith("plugin:")) {
      const id = feedType.replace("plugin:", "");
      const raw = pluginNames[id] || id;
      // Plugin manifests carry no display name, so the raw `name` bleeds
      // through ("weather"). Title-case it rather than show an identifier
      // next to properly formatted shelf labels.
      return raw.charAt(0).toUpperCase() + raw.slice(1);
    }
    return t(FEED_LABELS[feedType] ?? feedType);
  };

  return (
    <Modal visible={visible} transparent animationType="fade" onRequestClose={onClose}>
      <View style={styles.backdrop}>
        <View style={[styles.sheet, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.lg }]}>
          <Text style={[styles.title, { color: theme.colors.text }]}>
            {t("media.home.customizeFeed", "Customize Feed")}
          </Text>
          <Text style={[styles.hint, { color: theme.colors.textMuted }]}>
            {t("media.home.customizeHintTv", "Press OK on a row to show or hide it")}
          </Text>

          <ScrollView style={styles.list}>
            {rows.map((row, i) => (
              <View key={row.feedType} style={[styles.row, { borderBottomColor: theme.colors.border }]}>
                <FocusableCard
                  hasTVPreferredFocus={i === 0}
                  onPress={() => apply(toggleRow(rows, i))}
                  accessibilityLabel={label(row.feedType)}
                  accessibilityState={{ selected: row.enabled }}
                  style={styles.rowMain}
                  focusedStyle={{ backgroundColor: theme.colors.accent + "33" }}
                >
                  <Ionicons
                    name={row.enabled ? "eye-outline" : "eye-off-outline"}
                    size={24}
                    color={row.enabled ? theme.colors.accent : theme.colors.textMuted}
                  />
                  <Text
                    style={[
                      styles.rowLabel,
                      { color: row.enabled ? theme.colors.text : theme.colors.textMuted },
                    ]}
                    numberOfLines={1}
                  >
                    {label(row.feedType)}
                  </Text>
                </FocusableCard>
                <FocusableCard
                  onPress={() => apply(moveRow(rows, i, -1))}
                  disabled={i === 0}
                  accessibilityLabel={t("common.feeds.moveUp", "Move section up")}
                  style={styles.arrowBtn}
                  focusedStyle={{ backgroundColor: theme.colors.accent + "33" }}
                >
                  <Ionicons name="chevron-up" size={22} color={i === 0 ? theme.colors.border : theme.colors.text} />
                </FocusableCard>
                <FocusableCard
                  onPress={() => apply(moveRow(rows, i, 1))}
                  disabled={i === rows.length - 1}
                  accessibilityLabel={t("common.feeds.moveDown", "Move section down")}
                  style={styles.arrowBtn}
                  focusedStyle={{ backgroundColor: theme.colors.accent + "33" }}
                >
                  <Ionicons
                    name="chevron-down"
                    size={22}
                    color={i === rows.length - 1 ? theme.colors.border : theme.colors.text}
                  />
                </FocusableCard>
              </View>
            ))}
          </ScrollView>

          <View style={styles.footer}>
            <FocusableCard
              onPress={() => void handleReset()}
              disabled={resetLocked}
              accessibilityLabel={t(confirmingReset ? "common.feeds.resetConfirm" : "common.feeds.resetDefaults")}
              style={[styles.resetBtn, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
              focusedStyle={{ backgroundColor: theme.colors.accent + "33" }}
            >
              <Ionicons name="refresh-outline" size={20} color={theme.colors.textMuted} />
              <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: FONT_SIZE.base }}>
                {t(confirmingReset ? "common.feeds.resetConfirm" : "common.feeds.resetDefaults")}
              </Text>
            </FocusableCard>

            <FocusableCard
              onPress={onClose}
              // Same reason as the playlist sheet: with no row to focus this is
              // the only focusable left, and an unfocused TV screen stops
              // receiving key events altogether.
              hasTVPreferredFocus={rows.length === 0}
              accessibilityLabel={t("common.close", "Close")}
              style={[styles.close, { backgroundColor: theme.colors.surfaceRaised, borderRadius: theme.radii.md }]}
            >
              <Text style={{ color: theme.colors.text, fontWeight: "600", fontSize: FONT_SIZE.lg }}>
                {t("common.close", "Close")}
              </Text>
            </FocusableCard>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: { flex: 1, justifyContent: "center", alignItems: "center", backgroundColor: "rgba(0,0,0,0.7)" },
  sheet: { width: "60%", maxHeight: "80%", padding: 32 },
  title: { fontSize: FONT_SIZE["2xl"], fontWeight: "700" },
  hint: { fontSize: FONT_SIZE.base, marginTop: 8, marginBottom: 16 },
  list: { flexGrow: 0 },
  row: { flexDirection: "row", alignItems: "center", borderBottomWidth: StyleSheet.hairlineWidth },
  rowMain: { flex: 1, flexDirection: "row", alignItems: "center", gap: 12, paddingVertical: 10, paddingHorizontal: 12 },
  rowLabel: { flex: 1, fontSize: FONT_SIZE.lg },
  arrowBtn: { paddingVertical: 10, paddingHorizontal: 12 },
  footer: { flexDirection: "row", justifyContent: "space-between", alignItems: "center", marginTop: 20 },
  resetBtn: { flexDirection: "row", alignItems: "center", gap: 8, paddingVertical: 10, paddingHorizontal: 20 },
  close: { paddingVertical: 10, paddingHorizontal: 24, alignItems: "center" },
});
