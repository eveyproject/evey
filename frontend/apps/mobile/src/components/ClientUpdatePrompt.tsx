import { View, Text, Modal, Pressable, StyleSheet } from "react-native";
import { usePathname } from "expo-router";
import { useTranslation } from "react-i18next";
import * as Updates from "expo-updates";
import { FONT_SIZE } from "@evey/theme";
import { useClientUpdates } from "@evey/client/native/useClientUpdates";
import { useTheme } from "../providers/ThemeProvider";
import { clientUpdateBinding } from "../lib/clientUpdates";
import appConfig from "../../app.json";

const APP_VERSION: string = appConfig.expo.version;

/**
 * Offers a restart once the server's bundle has been downloaded and is
 * another app version than the one running. Same version: nothing the viewer
 * would notice, it starts with the next launch without asking. "Later" costs
 * nothing either. Never over the player, the guest one included.
 */
export function ClientUpdatePrompt() {
  const { t } = useTranslation();
  const theme = useTheme();
  const pathname = usePathname();
  const { isUpdatePending, downloadedUpdate } = Updates.useUpdates();
  const downloaded = downloadedUpdate?.manifest as { extra?: { expoClient?: { version?: string } } } | undefined;
  const isOtherVersion = downloaded?.extra?.expoClient?.version !== APP_VERSION;
  const { offerRestart, dismiss } = useClientUpdates(clientUpdateBinding, isUpdatePending && isOtherVersion);
  if (!offerRestart || pathname?.startsWith("/player") || pathname?.startsWith("/watch")) return null;

  // A reload that fails leaves the app as it was: the bundle still starts
  // with the next launch, so the offer simply goes away.
  const restart = () => {
    Updates.reloadAsync().catch((error: unknown) => {
      clientUpdateBinding.onError?.(error);
      dismiss();
    });
  };

  return (
    <Modal visible transparent animationType="fade" onRequestClose={dismiss}>
      <View style={[styles.backdrop, { backgroundColor: theme.colors.overlay }]}>
        <View style={[styles.card, { backgroundColor: theme.colors.surface, borderColor: theme.colors.border }]} testID="client-update-prompt">
          <Text style={[styles.title, { color: theme.colors.text }]}>{t("common.clientUpdate.title", "Update ready")}</Text>
          <Text style={[styles.body, { color: theme.colors.textMuted }]}>
            {t("common.clientUpdate.body", "A new version of the app was downloaded from your server.")}
          </Text>
          <View style={styles.actions}>
            <Pressable
              onPress={dismiss}
              accessibilityRole="button"
              style={[styles.button, { backgroundColor: theme.colors.bg }]}
              testID="client-update-later"
            >
              <Text style={[styles.laterText, { color: theme.colors.text }]}>{t("common.clientUpdate.later", "Later")}</Text>
            </Pressable>
            <Pressable
              onPress={restart}
              accessibilityRole="button"
              style={[styles.button, { backgroundColor: theme.colors.accent }]}
              testID="client-update-restart"
            >
              <Text style={[styles.restartText, { color: theme.colors.onAccent }]}>{t("common.clientUpdate.restart", "Restart")}</Text>
            </Pressable>
          </View>
        </View>
      </View>
    </Modal>
  );
}

const styles = StyleSheet.create({
  backdrop: {
    flex: 1,
    alignItems: "center",
    justifyContent: "center",
    padding: 24,
  },
  card: {
    alignSelf: "stretch",
    borderRadius: 16,
    borderWidth: 1,
    padding: 20,
  },
  title: {
    fontSize: FONT_SIZE.lg,
    fontWeight: "700",
  },
  body: {
    fontSize: FONT_SIZE.sm,
    marginTop: 8,
    marginBottom: 20,
  },
  actions: {
    flexDirection: "row",
    gap: 12,
  },
  button: {
    flex: 1,
    borderRadius: 10,
    paddingHorizontal: 12,
    paddingVertical: 12,
    alignItems: "center",
    justifyContent: "center",
  },
  restartText: {
    fontWeight: "700",
    fontSize: FONT_SIZE.sm,
  },
  laterText: {
    fontWeight: "600",
    fontSize: FONT_SIZE.sm,
  },
});
