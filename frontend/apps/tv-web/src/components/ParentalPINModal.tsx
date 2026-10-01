import { useState } from "react";
import { useTranslation } from "react-i18next";
import { configureParentalUnlock, markUnlocked, verifyParentalPin } from "@evey/client";
import { FocusRoot, useBackHandler } from "../tv/focus";
import { Pressable, Spinner } from "./ui";
import { Icon } from "./icons";

// A television's remote is shared by the whole living room: an unlock is
// forgotten after half an hour, as on the Android television.
configureParentalUnlock({ ttlMs: 30 * 60 * 1000 });

const PIN_LENGTH = 4;
const KEYPAD_ROWS: (string | null)[][] = [
  ["1", "2", "3"],
  ["4", "5", "6"],
  ["7", "8", "9"],
  [null, "0", "del"],
];

interface Props {
  profileId: string;
  // The profile being left; @evey/client's verifyParentalPin says why it is
  // asked before the target.
  fallbackProfileId?: string;
  // The server's short-lived proof, which a profile switch must carry.
  onVerified: (pinToken?: string) => void;
  onClose: () => void;
  initialError?: string | null;
  /** Overrides for another use of the keypad (the kids opening-hours lock). */
  title?: string;
  description?: string;
  /** Checks the PIN itself: resolves to an error to show, or null on success. */
  submitPin?: (pin: string) => Promise<string | null>;
}

/** The Android television's PIN dialog: four dots and a keypad the remote walks. */
export function ParentalPINModal({ profileId, fallbackProfileId, onVerified, onClose, initialError, title, description, submitPin }: Props) {
  const { t } = useTranslation();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(initialError ?? null);
  const [loading, setLoading] = useState(false);
  useBackHandler(onClose);

  async function submit(candidate: string) {
    setLoading(true);
    setError(null);
    if (submitPin) {
      const err = await submitPin(candidate);
      setLoading(false);
      if (err === null) onVerified();
      else {
        setError(err);
        setPin("");
      }
      return;
    }
    try {
      const pinToken = await verifyParentalPin({ targetProfileId: profileId, leavingProfileId: fallbackProfileId, pin: candidate });
      if (pinToken !== null) {
        markUnlocked(profileId);
        onVerified(pinToken || undefined);
      } else {
        setError(t("common.parental.pinIncorrect", "Incorrect PIN"));
        setPin("");
      }
    } catch {
      setError(t("common.loadError", "Failed to load"));
      setPin("");
    } finally {
      setLoading(false);
    }
  }

  function press(key: string) {
    if (loading) return;
    if (error) setError(null);
    if (key === "del") {
      setPin((p) => p.slice(0, -1));
      return;
    }
    if (pin.length >= PIN_LENGTH) return;
    const next = pin + key;
    setPin(next);
    if (next.length === PIN_LENGTH) void submit(next);
  }

  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus boundary className="pin-dialog">
        <h2 className="pin-title">{title ?? t("common.parental.restrictedContent", "Restricted Content")}</h2>
        <p className="pin-desc">{description ?? t("common.parental.pinRequired", "Enter your parental PIN to continue")}</p>
        <div className="pin-dots" aria-label={t("media.parental.pinDigitsEntered", { count: pin.length, total: PIN_LENGTH })}>
          {Array.from({ length: PIN_LENGTH }, (_, i) => (
            <span key={i} className={`pin-dot ${i < pin.length ? "filled" : ""} ${error ? "error" : ""}`} />
          ))}
        </div>
        <div className="pin-status">{loading ? <Spinner /> : error ? <span className="pin-error">{error}</span> : null}</div>
        <div className="keypad">
          {KEYPAD_ROWS.map((row, ri) => (
            <div key={ri} className="keypad-row">
              {row.map((key, ki) =>
                key === null ? (
                  <span key={ki} className="key" />
                ) : (
                  <Pressable
                    key={ki}
                    className="tv-card key"
                    autoFocus={ri === 0 && ki === 0}
                    label={key === "del" ? t("common.parental.delete", "Delete") : key}
                    onPress={() => press(key)}
                  >
                    {key === "del" ? <Icon name="backspace-outline" size={24} /> : key}
                  </Pressable>
                ),
              )}
            </div>
          ))}
        </div>
        <Pressable className="tv-card pin-cancel" onPress={onClose}>{t("common.cancel", "Cancel")}</Pressable>
      </FocusRoot>
    </div>
  );
}
