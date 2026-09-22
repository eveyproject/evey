import { useState } from "react";
import { useTranslation } from "react-i18next";
import { verifyParentalPin } from "@evey/client";
import { FocusRoot } from "../tv/focus";
import { Button, TextField } from "../components/ui";

const PIN_LENGTH = 4;

interface Props {
  profileId: string;
  profileName: string;
  // The profile being left; @evey/client's verifyParentalPin says why it is
  // asked before the target.
  fallbackProfileId?: string;
  // The short-lived server proof verify-pin mints; the caller hands it to
  // the profile switch, which the server otherwise refuses with a 403.
  onVerified: (pinToken?: string) => void;
  onCancel: () => void;
  // An error from the caller (the switch refused the proof): shown at open,
  // the prompt itself is mounted fresh each time.
  initialError?: string | null;
}

// The parental PIN, asked before a protected profile opens: four digits
// typed on the remote's keyboard, checked by the server, never compared
// here. Same endpoint and same rule as the Android TV modal.
export function PinPrompt({ profileId, profileName, fallbackProfileId, onVerified, onCancel, initialError }: Props) {
  const { t } = useTranslation();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(initialError ?? null);
  const [busy, setBusy] = useState(false);

  async function submit() {
    if (pin.length < PIN_LENGTH || busy) return;
    setBusy(true);
    setError(null);
    try {
      // The order and the network rule live in @evey/client.
      const pinToken = await verifyParentalPin({ targetProfileId: profileId, leavingProfileId: fallbackProfileId, pin });
      if (pinToken !== null) onVerified(pinToken || undefined);
      else setError(t("common.parental.pinIncorrect", "Incorrect PIN"));
    } catch {
      setError(t("common.loadError", "Failed to load"));
    } finally {
      setBusy(false);
      setPin("");
    }
  }

  return (
    <div className="modal">
      <FocusRoot autoFocus className="modal-card">
        <h2 className="title">{profileName}</h2>
        <p className="muted">{t("common.parental.pinRequired", "This content requires parental PIN verification.")}</p>
        <TextField value={pin} onChange={(v) => setPin(v.replace(/\D/g, "").slice(0, PIN_LENGTH))} placeholder={t("common.parental.pinLabel", "PIN (4 digits)")} type="password" autoFocus onSubmit={() => void submit()} />
        {error ? <p className="error">{error}</p> : null}
        <div className="actions">
          <Button variant="primary" disabled={busy || pin.length < PIN_LENGTH} onPress={() => void submit()}>{t("common.parental.validate", "Validate")}</Button>
          <Button onPress={onCancel}>{t("common.cancel", "Cancel")}</Button>
        </div>
      </FocusRoot>
    </div>
  );
}
