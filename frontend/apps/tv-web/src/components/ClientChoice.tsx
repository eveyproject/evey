import { useTranslation } from "react-i18next";
import { rememberChoice } from "../lib/clientChoice";
import { FocusRoot } from "../tv/focus";
import { Button } from "./ui";

// Asked once, on a served page, when nothing says which client was meant.
// Rendered INSTEAD of the application, not over it: measured in the LG
// simulator, the screen behind autofocuses its own field and steals the ring,
// so a remote could not answer. "Stay here" holds the focus: one press of OK.
export function ClientChoice({ onAnswered }: { onAnswered: () => void }) {
  const { t } = useTranslation();
  return (
    <div className="modal">
      <FocusRoot autoFocus className="modal-card">
        <h2 className="title">{t("common.clientChoice.title", "Television interface")}</h2>
        <p className="muted">{t("common.clientChoice.body", "This page is made for a television and its remote control. On a computer, the web version is the comfortable one.")}</p>
        <div className="actions">
          <Button variant="primary" autoFocus onPress={() => { rememberChoice("tv"); onAnswered(); }}>
            {t("common.clientChoice.stay", "Stay here")}
          </Button>
          <Button onPress={() => { rememberChoice("web"); window.location.replace("/?from=tv"); }}>
            {t("common.clientChoice.openWeb", "Open the web version")}
          </Button>
        </div>
      </FocusRoot>
    </div>
  );
}
