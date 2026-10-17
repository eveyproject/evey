import { View, Text, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import type { V1DiskIOInfo } from "@evey/api";
import type { useTheme } from "../../providers/ThemeProvider";

type Theme = ReturnType<typeof useTheme>;

// formatBytesPerSecond renders a byte rate the way an admin reads iostat
// (mirror of the web helper).
export function formatBytesPerSecond(bps: string | number | undefined): string {
  const n = typeof bps === "string" ? parseInt(bps, 10) : (bps ?? 0);
  if (!Number.isFinite(n) || n < 0) return "-";
  if (n >= 1e9) return `${(n / 1e9).toFixed(2)} GB/s`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB/s`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)} kB/s`;
  return `${n} B/s`;
}

// busiestDisk is the disk that waited the most, not the busiest by share: a
// disk 100 % busy serving requests in 2 ms keeps up, one 60 % busy making each
// request wait 40 ms is the one that broke the load test.
export function busiestDisk(disks: V1DiskIOInfo[] | undefined): V1DiskIOInfo | undefined {
  if (!disks || disks.length === 0) return undefined;
  return disks.reduce((worst, d) => ((d.awaitMs ?? 0) > (worst.awaitMs ?? 0) ? d : worst), disks[0]);
}

interface DiskIOListProps {
  disks: V1DiskIOInfo[];
  idleAwaitMs: number | undefined;
  saturationAwaitMs: number | undefined;
  iowaitPercent: number | undefined;
  theme: Theme;
}

// DiskIOList mirrors the web system-health detail: one row per physical disk,
// the wait first, then the busy share and the rates, with the reference waits
// the verdict comes from.
export function DiskIOList({ disks, idleAwaitMs, saturationAwaitMs, iowaitPercent, theme }: DiskIOListProps) {
  const { t } = useTranslation();
  return (
    <View style={[styles.card, { backgroundColor: theme.colors.surface, borderRadius: theme.radii.md }]} testID="disk-io-list">
      <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
        {t("admin.systemHealth.diskRestLine", { idle: (idleAwaitMs ?? 0).toFixed(1), line: (saturationAwaitMs ?? 0).toFixed(0) })}
      </Text>
      <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
        {t("admin.systemHealth.diskIowait", { percent: (iowaitPercent ?? 0).toFixed(1) })}
      </Text>
      {disks.map((d) => {
        const saturated = d.saturated === true;
        return (
          <View key={d.device ?? d.label} style={[styles.row, { borderTopColor: theme.colors.border }]} testID="disk-io-row">
            <View style={styles.head}>
              <View style={{ flex: 1, minWidth: 0 }}>
                <Text style={{ color: theme.colors.text, fontSize: 13, fontWeight: "600" }} numberOfLines={1}>{d.label || d.device}</Text>
                <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                  {d.device}{d.rotational === false ? ` · ${t("admin.systemHealth.diskSolidState")}` : ""}
                </Text>
              </View>
              <Text style={{ color: saturated ? theme.colors.danger : theme.colors.text, fontSize: 18, fontWeight: "700" }}>
                {(d.awaitMs ?? 0).toFixed(1)} ms
              </Text>
            </View>
            <View style={styles.facts}>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>{t("admin.systemHealth.diskUtil", { percent: Math.round(d.utilPercent ?? 0) })}</Text>
              <Text style={{ color: theme.colors.textMuted, fontSize: 11 }}>
                {t("admin.systemHealth.diskRates", { read: formatBytesPerSecond(d.readBps), write: formatBytesPerSecond(d.writeBps) })}
              </Text>
              {saturated && (
                <View style={styles.badge}>
                  <Ionicons name="warning-outline" size={12} color={theme.colors.danger} />
                  <Text style={{ color: theme.colors.danger, fontSize: 11, fontWeight: "600" }}>{t("admin.systemHealth.diskSaturated")}</Text>
                </View>
              )}
            </View>
          </View>
        );
      })}
      <Text style={{ color: theme.colors.textMuted, fontSize: 11, marginTop: 8 }}>{t("admin.systemHealth.diskMachineNote")}</Text>
    </View>
  );
}

const styles = StyleSheet.create({
  card: { padding: 16, gap: 4 },
  row: { paddingTop: 10, marginTop: 8, borderTopWidth: StyleSheet.hairlineWidth, gap: 4 },
  head: { flexDirection: "row", alignItems: "center", gap: 12 },
  facts: { flexDirection: "row", flexWrap: "wrap", gap: 10, alignItems: "center" },
  badge: { flexDirection: "row", alignItems: "center", gap: 4 },
});
