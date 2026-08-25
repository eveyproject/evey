import { View, Text, Pressable, ActivityIndicator, StyleSheet } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import { FONT_SIZE, ICON_SIZE } from "@evey/theme";
import { useTheme } from "../../providers/ThemeProvider";
import { BottomSheet } from "../BottomSheet";
import { usePlayOnDevices, usePlayOnCommand, type PlayOnDevice } from "../../hooks/usePlayOnDevices";

interface Props {
  visible: boolean;
  mediaId: string;
  onClose: () => void;
}

function iconFor(type: string): keyof typeof Ionicons.glyphMap {
  switch (type) {
    case "tv": return "tv-outline";
    case "mobile": return "phone-portrait-outline";
    default: return "desktop-outline";
  }
}

// Mirrors apps/web/src/features/playback/RemotePlaybackMenu.tsx's Evey-device
// section: send this media to one of the user's other connected clients
// (loadAndPlay command over the same server-brokered device WebSocket
// useDeviceRegistration.ts registers into).
export function PlayOnSheet({ visible, mediaId, onClose }: Props) {
  const { t } = useTranslation();
  const theme = useTheme();
  const { data: devices, isLoading, refetch } = usePlayOnDevices({ enabled: visible });
  const sendCommand = usePlayOnCommand();

  function handleSelect(device: PlayOnDevice) {
    sendCommand.mutate(
      { deviceId: device.id, command: { type: "loadAndPlay", mediaId } },
      {
        onSuccess: () => {
          toast.success(t("player.playOn.sent", { name: device.name }));
          onClose();
        },
        onError: () => {
          // Wording already covers the disconnected case ("the device may
          // have disconnected"), same message the web menu shows for any
          // send failure including a 404 - so no separate key is needed.
          toast.error(t("player.playOn.failed"));
          void refetch();
        },
      },
    );
  }

  const hasDevices = (devices?.length ?? 0) > 0;

  return (
    <BottomSheet visible={visible} onClose={onClose} title={t("player.playOn.title")}>
      {isLoading ? (
        <ActivityIndicator color={theme.colors.accent} style={{ marginVertical: 24 }} />
      ) : hasDevices ? (
        <View>
          {devices!.map((d) => (
            <Pressable
              key={d.id}
              onPress={() => handleSelect(d)}
              disabled={sendCommand.isPending}
              style={[
                styles.row,
                { borderBottomColor: theme.colors.border, opacity: sendCommand.isPending ? 0.5 : 1 },
              ]}
              accessible={true}
              accessibilityLabel={d.name}
              accessibilityRole="button"
            >
              <Ionicons name={iconFor(d.type)} size={ICON_SIZE.lg} color={theme.colors.accent} />
              <Text style={[styles.name, { color: theme.colors.text }]} numberOfLines={1}>
                {d.name}
              </Text>
            </Pressable>
          ))}
        </View>
      ) : (
        <Text style={[styles.empty, { color: theme.colors.textMuted }]}>
          {t("player.playOn.empty")}
        </Text>
      )}
    </BottomSheet>
  );
}

const styles = StyleSheet.create({
  row: {
    flexDirection: "row",
    alignItems: "center",
    gap: 12,
    paddingVertical: 14,
    borderBottomWidth: StyleSheet.hairlineWidth,
  },
  name: { flex: 1, fontSize: FONT_SIZE.sm, fontWeight: "500" },
  empty: { fontSize: FONT_SIZE.sm, textAlign: "center", paddingVertical: 24 },
});
