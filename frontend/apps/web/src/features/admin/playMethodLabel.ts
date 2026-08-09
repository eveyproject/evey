import { useTranslation } from "react-i18next";
import { effectivePlayMethod, type PlayMethodInput } from "@evey/api";

// One label for both admin surfaces (the dashboard's Now Playing card and the
// full sessions list), which used to carry byte-identical copies of this switch
// and drifted: the dashboard grew a wasm correction the sessions list never
// got, so the same session read "Remux" in one panel and "Transcodage" in the
// other.
//
// Reads the EFFECTIVE method, not the one the client declared at start: a
// session whose tracks are all copied is a remux however it was announced.
// Returns "" when there is no method to name (cast-resurrected and music
// sessions carry none) so the caller drops the badge instead of rendering a
// blank pill.
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
