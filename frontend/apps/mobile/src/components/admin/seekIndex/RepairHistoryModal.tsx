import { View, Text, Pressable, ActivityIndicator, ScrollView, Modal, StyleSheet } from "react-native";
import { useTranslation } from "react-i18next";
import {
  useAdminMediaServiceGetMediaRepairHistory,
  useAdminMediaServiceUndoMediaRepair,
} from "@evey/api";
import { useTheme } from "../../../providers/ThemeProvider";
import { repairOpLabel, repairEvidenceLine, repairShiftSummary } from "@evey/client";

// What was done to this file's bytes, and the way back (mirror web
// RepairHistoryPanel). Evey keeps no copy of a repaired file: each entry
// records the transformation precisely enough to be replayed backwards, so
// "undo" reads the entry rather than a backup. The evidence sits next to the
// action - acting on a measurement and acting on the file's own claim are not
// the same thing.


function outcomeColor(outcome: string | undefined, theme: ReturnType<typeof useTheme>): string {
  switch (outcome) {
    case "applied": return theme.colors.success;
    case "rolled_back": return theme.colors.warning;
    case "failed": return theme.colors.error;
    default: return theme.colors.textMuted;
  }
}


export function RepairHistoryModal({ mediaId, title, onClose }: { mediaId: string; title: string; onClose: () => void }) {
  const theme = useTheme();
  const { t } = useTranslation();
  const s = styles(theme);
  const history = useAdminMediaServiceGetMediaRepairHistory(mediaId, { limit: 20 });
  const undo = useAdminMediaServiceUndoMediaRepair({ mutation: { onSuccess: () => void history.refetch() } });
  const entries = history.data?.entries ?? [];
  const undoable = entries.find((e) => e.reversible);

  return (
    <Modal visible transparent animationType="slide" onRequestClose={onClose}>
      <Pressable style={{ flex: 1, backgroundColor: "rgba(0,0,0,0.5)" }} onPress={onClose} />
      <View style={[s.sheet, { backgroundColor: theme.colors.surface, borderTopLeftRadius: theme.radii.lg, borderTopRightRadius: theme.radii.lg }]}>
        <View style={{ flexDirection: "row", justifyContent: "space-between", alignItems: "center", gap: 8 }}>
          <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700", flex: 1 }} numberOfLines={1}>
            {t("admin.repairHistory.title", { title })}
          </Text>
          <Pressable onPress={onClose}><Text style={{ color: theme.colors.accent, fontWeight: "600" }}>{t("common.close")}</Text></Pressable>
        </View>
        <Text style={s.hint}>{t("admin.repairHistory.intro", "Each repair recorded precisely enough to be undone.")}</Text>

        <ScrollView style={{ maxHeight: 360, marginTop: 8 }}>
          {history.isLoading ? (
            <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 16 }} />
          ) : entries.length === 0 ? (
            <Text style={s.hint}>{t("admin.repairHistory.empty", "No repair history for this file.")}</Text>
          ) : (
            entries.map((e, i) => (
              <View key={i} style={[s.entry, { borderTopColor: theme.colors.border }]}>
                <View style={{ flexDirection: "row", flexWrap: "wrap", alignItems: "center", gap: 6 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 12 }}>{new Date(Number(e.atMs ?? 0)).toLocaleString()}</Text>
                  <Text style={{ color: theme.colors.accent, fontSize: 11, fontWeight: "600" }}>{repairOpLabel(e.op, t)}</Text>
                  <Text style={{ color: outcomeColor(e.outcome, theme), fontSize: 11, fontWeight: "600" }}>{t(`admin.repairHistory.outcome.${e.outcome}`)}</Text>
                  {e.reversible ? <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t("admin.repairHistory.reversible", "reversible")}</Text> : null}
                </View>
                {repairShiftSummary(e) ? <Text style={s.hint}>{t("admin.repairHistory.shift", { shift: repairShiftSummary(e) })}</Text> : null}
                <Text style={s.hint}>
                  {repairEvidenceLine(e, t)}
                </Text>
                {e.error ? <Text style={{ color: theme.colors.error, fontSize: 11 }}>{e.error}</Text> : null}
              </View>
            ))
          )}
        </ScrollView>

        {undoable ? (
          <View style={{ marginTop: 10, borderTopWidth: StyleSheet.hairlineWidth, borderTopColor: theme.colors.border, paddingTop: 10 }}>
            <Pressable
              onPress={() => undo.mutate({ mediaId, data: {} })}
              disabled={undo.isPending}
              style={{ borderWidth: 1, borderColor: theme.colors.border, borderRadius: theme.radii.md, paddingVertical: 12, alignItems: "center", opacity: undo.isPending ? 0.5 : 1 }}
            >
              {undo.isPending ? <ActivityIndicator size="small" color={theme.colors.text} /> : <Text style={{ color: theme.colors.text, fontWeight: "600" }}>{t("admin.repairHistory.undoBtn", "Undo last repair")}</Text>}
            </Pressable>
            <Text style={s.hint}>{t("admin.repairHistory.undoHint", "Replays the transformation backwards from the journal.")}</Text>
          </View>
        ) : entries.length > 0 ? (
          <Text style={[s.hint, { marginTop: 8 }]}>{t("admin.repairHistory.notReversible", "No reversible repair to undo.")}</Text>
        ) : null}
      </View>
    </Modal>
  );
}

const styles = (theme: ReturnType<typeof useTheme>) =>
  StyleSheet.create({
    sheet: { position: "absolute", left: 0, right: 0, bottom: 0, padding: 16, paddingBottom: 28, maxHeight: "82%" },
    hint: { color: theme.colors.textMuted, fontSize: 12, marginTop: 2 },
    entry: { borderTopWidth: StyleSheet.hairlineWidth, paddingTop: 6, marginTop: 6 },
  });
