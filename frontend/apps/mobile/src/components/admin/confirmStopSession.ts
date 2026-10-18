import { Alert } from "react-native";
import type { TFunction } from "i18next";
import type { V1ActiveSession } from "@evey/api";

// Stopping someone's playback is not undoable: the Sessions screen and the
// dashboard's Now playing both ask first, like the web arms its button before
// the second tap.
export function confirmStopSession(t: TFunction, session: V1ActiveSession, stop: () => void): void {
  Alert.alert(
    t("admin.sessions.stopSession", "Stop Session"),
    `${t("admin.sessions.stop", "Stop")} ${session.profileName}?`,
    [
      { text: t("common.cancel"), style: "cancel" },
      { text: t("admin.sessions.stop", "Stop"), style: "destructive", onPress: stop },
    ],
  );
}
