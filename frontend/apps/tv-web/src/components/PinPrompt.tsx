import { useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Button, TextField } from "../components/ui";

const PIN_LENGTH = 4;

interface Props {
  profileId: string;
  profileName: string;
  // The profile being left. The server lets a restricted profile be left
  // on the PIN of the profile being left or on the target's own. The PIN
  // is checked against the leaving profile FIRST: it is the one known to
  // hold a PIN, whereas verify-pin on a PIN-less target says "valid" for
  // any digits and mints a proof the switch then refuses (dev server,
  // 2026-09-22: 9999 accepted by the target, the modal closed, the switch
  // failed silently). The target is tried second, for a target with a PIN.
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

  const verifyAgainst = async (id: string): Promise<{ valid: boolean; pinToken?: string } | null> => {
    try {
      return await eveyFetch<{ valid: boolean; pinToken?: string }>({ url: `/api/v1/profiles/${id}/parental/verify-pin`, method: "POST", data: { pin } });
    } catch {
      return null;
    }
  };

  async function submit() {
    if (pin.length < PIN_LENGTH || busy) return;
    setBusy(true);
    setError(null);
    try {
      let res = fallbackProfileId && fallbackProfileId !== profileId ? await verifyAgainst(fallbackProfileId) : null;
      if (!res?.valid) res = await verifyAgainst(profileId);
      if (res?.valid) onVerified(res.pinToken);
      else setError(t("common.parental.pinIncorrect", "Incorrect PIN"));
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
