import { useTranslation } from "react-i18next";
import { effectivePlayMethod, type PlayMethodInput } from "@evey/api";

// Shared by the Now Playing card and the sessions list: two copies of this
// switch had already drifted on the wasm case. Returns "" when there is no
// method to name, so the caller drops the badge.
export function useSessionPlayMethodLabel() {
  const { t } = useTranslation();
  return (session: PlayMethodInput): string => {
    switch (effectivePlayMethod(session)) {
      case "direct_play":
        return t("admin.sessions.directPlay");
      case "direct_stream":
        return t("admin.sessions.directStream");
      case "transcode":
        return t("admin.sessions.transcode");
      default:
        return "";
    }
  };
}
