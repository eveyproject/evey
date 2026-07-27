import { Text, Pressable, StyleSheet, ActivityIndicator, View } from "react-native";
import { useTranslation } from "react-i18next";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import { eveyFetch } from "@evey/api";
import type { V1User } from "@evey/api";
import { toast } from "@evey/store";
import { BottomSheet } from "../../../../src/components/BottomSheet";
import { useTheme } from "../../../../src/providers/ThemeProvider";
import { FONT_SIZE } from "@evey/theme";

const BITRATE_OPTIONS = [
  { label: "Unlimited", value: 0 },
  { label: "1 Mbps", value: 1000 },
  { label: "2 Mbps", value: 2000 },
  { label: "4 Mbps", value: 4000 },
  { label: "8 Mbps", value: 8000 },
  { label: "20 Mbps", value: 20000 },
  { label: "40 Mbps", value: 40000 },
];

interface BitrateModalProps {
  user: V1User;
  visible: boolean;
  onClose: () => void;
}

export function BitrateModal({ user, visible, onClose }: BitrateModalProps) {
  const { t } = useTranslation();
  const theme = useTheme();
  const qc = useQueryClient();

  const queryKey = ["/api/v1/admin/users", user.id, "bitrate-limit"];
  const { data: currentLimit, isLoading } = useQuery<{ max_bitrate_kbps: number }>({
    queryKey,
    queryFn: () =>
      eveyFetch<{ max_bitrate_kbps: number }>({
        url: `/api/v1/admin/users/${user.id}/bitrate-limit`,
        method: "GET",
      }),
    enabled: visible && !!user.id,
  });

  async function handleSelect(value: number) {
    try {
      await eveyFetch<void>({
        url: `/api/v1/admin/users/${user.id}/bitrate-limit`,
        method: "PATCH",
        data: { max_bitrate_kbps: value },
      });
      await qc.invalidateQueries({ queryKey });
      toast.success(t("admin.users.bitrateUpdated", "Bitrate limit updated"));
      onClose();
    } catch {
      toast.error(t("admin.users.failedBitrate", "Failed to update bitrate limit"));
    }
  }

  const activeValue = currentLimit?.max_bitrate_kbps ?? 0;

  return (
    <BottomSheet
      visible={visible}
      onClose={onClose}
      title={t("admin.users.maxBitrateTitle", { name: user.username, defaultValue: `Max bitrate - ${user.username ?? ""}` })}
    >
      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ paddingVertical: 24 }} />
      ) : (
        <View>
          {BITRATE_OPTIONS.map((opt) => {
            const active = activeValue === opt.value;
            return (
              <Pressable
                key={opt.value}
                style={[styles.row, { borderBottomColor: theme.colors.border }]}
                onPress={() => handleSelect(opt.value)}
                accessibilityRole="button"
                accessibilityLabel={opt.label}
              >
                <Text style={[styles.label, { color: theme.colors.text }, active && { color: theme.colors.accent }]}>
                  {opt.label}
                </Text>
                {active && <Text style={[styles.check, { color: theme.colors.accent }]}>{"✓"}</Text>}
              </Pressable>
            );
          })}
        </View>
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  label: {
    fontSize: FONT_SIZE.sm,
    flex: 1,
  },
  check: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "600",
    marginLeft: 12,
  },
});
