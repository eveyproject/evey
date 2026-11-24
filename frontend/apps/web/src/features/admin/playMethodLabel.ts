import { useTranslation } from "react-i18next";
import { describeSession, type V1ActiveSession } from "@evey/api";

// Shared by the Now Playing card and the sessions list. The method is the
// served path's (describeSession): the file itself is a direct play, copied
// samples a remux, any encoded track a transcode. Returns "" before any byte
// was served, so the caller drops the badge.
export function useSessionPlayMethodLabel() {
  const { t } = useTranslation();
  return (session: Pick<V1ActiveSession, "servedVideo" | "servedAudio" | "played">): string => {
    switch (describeSession(session).method) {
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
