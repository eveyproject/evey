import { Modal, View, Text, FlatList, Pressable } from "react-native";
import Ionicons from "@expo/vector-icons/Ionicons";
import { useTranslation } from "react-i18next";
import { useTheme } from "../../providers/ThemeProvider";

export interface ServerDir {
  name: string;
  path: string;
}

interface Props {
  visible: boolean;
  dirs: ServerDir[];
  onSelect: (path: string) => void;
  onClose: () => void;
}

// Picks a library path from the server-side directory listing
// (GET /api/v1/admin/fs/browse) instead of free-typing it - the web's
// LibraryForm offers the same list in a select. Free text stays available in
// the form itself; this is a shortcut, not a constraint.
export function DirectoryPickerModal({ visible, dirs, onSelect, onClose }: Props) {
  const theme = useTheme();
  const { t } = useTranslation();

  return (
    <Modal visible={visible} transparent animationType="slide" onRequestClose={onClose}>
      <View style={{ flex: 1, justifyContent: "flex-end", backgroundColor: "rgba(0,0,0,0.5)" }}>
        <View
          style={{
            backgroundColor: theme.colors.bg,
            borderTopLeftRadius: theme.radii.md,
            borderTopRightRadius: theme.radii.md,
            maxHeight: "70%",
            paddingBottom: 24,
          }}
        >
          <View
            style={{
              flexDirection: "row",
              alignItems: "center",
              justifyContent: "space-between",
              padding: 16,
            }}
          >
            <Text style={{ color: theme.colors.text, fontSize: 16, fontWeight: "700" }}>
              {t("admin.libraries.selectDirectory", "Select a directory")}
            </Text>
            <Pressable
              onPress={onClose}
              hitSlop={8}
              accessible={true}
              accessibilityLabel={t("common.close", "Close")}
              accessibilityRole="button"
            >
              <Ionicons name="close" size={22} color={theme.colors.textMuted} />
            </Pressable>
          </View>
          <FlatList
            data={dirs}
            keyExtractor={(d) => d.path}
            ListEmptyComponent={
              <Text style={{ color: theme.colors.textMuted, fontSize: 14, textAlign: "center", padding: 24 }}>
                {t("admin.libraries.pathsHint", "No server directories found - type a path manually.")}
              </Text>
            }
            renderItem={({ item }) => (
              <Pressable
                testID={`dir-${item.path}`}
                onPress={() => {
                  onSelect(item.path);
                  onClose();
                }}
                style={{
                  flexDirection: "row",
                  alignItems: "center",
                  gap: 10,
                  paddingHorizontal: 16,
                  paddingVertical: 12,
                }}
                accessible={true}
                accessibilityLabel={item.path}
                accessibilityRole="button"
              >
                <Ionicons name="folder-outline" size={18} color={theme.colors.accent} />
                <View style={{ flex: 1 }}>
                  <Text style={{ color: theme.colors.text, fontSize: 14 }} numberOfLines={1}>
                    {item.name || item.path}
                  </Text>
                  <Text style={{ color: theme.colors.textMuted, fontSize: 12 }} numberOfLines={1}>
                    {item.path}
                  </Text>
                </View>
              </Pressable>
            )}
          />
        </View>
      </View>
    </Modal>
  );
}
