import { useState } from "react";
import { useTranslation } from "react-i18next";
import { parentalServiceVerifyPin, apiErrorMessage } from "@evey/api";
import type { V1Profile } from "@evey/api";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

interface Props {
  target: V1Profile;
  /**
   * Second profile whose PIN also authorises the switch. The server accepts
   * a proof from EITHER the target (when it carries a PIN) or the gated
   * SOURCE profile (when the target has none) - the client cannot tell which
   * one holds a PIN, so the same entry is tried against both. Verifying a
   * profile without a PIN simply fails and falls through.
   */
  fallback?: V1Profile;
  onCancel: () => void;
  // pinToken is the short-lived proof returned by verify-pin; it must be passed
  // to the profile switch (RefreshToken) to authorise leaving a kids profile.
  onVerified: (target: V1Profile, pinToken?: string) => void | Promise<void>;
}

// tryVerify returns the pinToken ("" when the server marked valid without
// minting one - legacy contract), null when this profile rejected the PIN or
// carries none (fall through to the fallback profile), and RETHROWS a real
// network failure (no httpStatus) so the dialog surfaces it instead of a
// misleading "Incorrect PIN".
async function tryVerify(profileId: string | undefined, pin: string): Promise<string | null> {
  if (!profileId) return null;
  try {
    const resp = await parentalServiceVerifyPin(profileId, { pin });
    if (!resp.valid) return null;
    return resp.pinToken ?? "";
  } catch (err) {
    if ((err as { httpStatus?: number }).httpStatus !== undefined) {
      return null; // server said no (no PIN on this profile, bad request) - fall through
    }
    throw err;
  }
}

export function PinPromptDialog({ target, fallback, onCancel, onVerified }: Props) {
  const { t } = useTranslation();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);

  const handleSubmit = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!target.id) return;
    if (!/^\d{4}$/.test(pin)) {
      setError(t("auth.profile.select.pinInvalid", "PIN must be 4 digits"));
      return;
    }
    setSubmitting(true);
    try {
      let pinToken = await tryVerify(target.id, pin);
      if (pinToken === null && fallback?.id && fallback.id !== target.id) {
        pinToken = await tryVerify(fallback.id, pin);
      }
      if (pinToken === null) {
        setError(t("auth.profile.select.pinWrong", "Incorrect PIN"));
        return;
      }
      await onVerified(target, pinToken || undefined);
    } catch (err) {
      setError(apiErrorMessage(err as Error, t, t("auth.profile.select.pinWrong", "Incorrect PIN")));
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal open onClose={onCancel} title={t("auth.profile.select.pinPromptTitle", "Enter PIN")}>
      <form onSubmit={handleSubmit} className="flex flex-col gap-4">
        <p className="text-sm text-[var(--text-muted)]">
          {fallback
            ? t("auth.profile.select.pinPromptBodyGeneric", "Enter the parental PIN to continue.")
            : t("auth.profile.select.pinPromptBody", "Enter the PIN for \"{{name}}\" to continue.", { name: target.name ?? "" })}
        </p>
        <Input
          type="password"
          inputMode="numeric"
          pattern="\d{4}"
          maxLength={4}
          value={pin}
          onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 4))}
          autoFocus
          aria-label={t("auth.profile.fields.pin", "PIN (4 digits)")}
          data-testid="profile-pin-prompt-input"
        />
        {error && (
          <p className="text-sm text-[var(--color-danger)]" data-testid="profile-pin-prompt-error" role="alert">{error}</p>
        )}
        <div className="flex justify-end gap-2 pt-2">
          <Button variant="ghost" type="button" onClick={onCancel}>
            {t("common.cancel")}
          </Button>
          <Button type="submit" loading={submitting} disabled={pin.length !== 4}>
            {t("common.confirm", "Confirm")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
