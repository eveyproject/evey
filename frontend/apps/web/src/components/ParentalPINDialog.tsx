import { useState, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Modal } from "@/components/ui/Modal";
import { Button } from "@/components/ui/Button";

interface ParentalPINDialogProps {
  open: boolean;
  onClose: () => void;
  onVerified: () => void;
  profileId: string;
}

export function ParentalPINDialog({ open, onClose, onVerified, profileId }: ParentalPINDialogProps) {
  const { t } = useTranslation();
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    if (open) {
      setPin("");
      setError(null);
      setTimeout(() => inputRef.current?.focus(), 50);
    }
  }, [open]);

  async function handleSubmit(e: React.FormEvent) {
    e.preventDefault();
    if (pin.length < 4) return;
    setLoading(true);
    setError(null);
    try {
      const { verifyParentalPin } = await import("@evey/client");
      const pinToken = await verifyParentalPin({ targetProfileId: profileId, pin });
      if (pinToken !== null) {
        onVerified();
      } else {
        setError(t("common.parental.pinIncorrect"));
        setPin("");
        inputRef.current?.focus();
      }
    } catch {
      // Request failed (network, 5xx, timeout) - not a wrong-PIN response.
      // Wrong-PIN is handled above via result.valid === false.
      setError(t("common.error"));
      setPin("");
      inputRef.current?.focus();
    } finally {
      setLoading(false);
    }
  }

  return (
    <Modal open={open} onClose={onClose} title={t("common.parental.restrictedContent")}>
      <p className="text-sm mb-4" style={{ color: "var(--text-muted)" }}>
        {t("common.parental.pinRequired")}
      </p>
      <form onSubmit={(e) => void handleSubmit(e)} className="flex flex-col gap-4">
        <div className="flex flex-col gap-1">
          <label htmlFor="parental-pin-input" className="text-sm font-medium" style={{ color: "var(--text)" }}>
            {t("common.parental.pinLabel")}
          </label>
          <input
            id="parental-pin-input"
            ref={inputRef}
            type="password"
            inputMode="numeric"
            pattern="[0-9]*"
            maxLength={4}
            value={pin}
            onChange={(e) => {
              const val = e.target.value.replace(/\D/g, "").slice(0, 4);
              setPin(val);
              if (error) setError(null);
            }}
            className="w-full px-3 py-2 rounded text-center text-2xl tracking-[0.5em] font-mono focus-visible:outline focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:[outline-color:var(--accent)]"
            style={{
              backgroundColor: "var(--surface-raised)",
              border: `1px solid ${error ? "var(--danger-bg, #ef4444)" : "var(--border)"}`,
              color: "var(--text)",
            }}
            placeholder="••••"
            autoComplete="off"
            data-testid="pin-input"
          />
          {error && (
            <p className="text-sm" style={{ color: "var(--danger-bg, #ef4444)" }} data-testid="pin-error">
              {error}
            </p>
          )}
        </div>
        <div className="flex gap-3">
          <Button type="submit" variant="primary" loading={loading} disabled={pin.length < 4} data-testid="pin-submit">
            {t("common.parental.validate")}
          </Button>
          <Button type="button" variant="secondary" onClick={onClose} data-testid="pin-cancel">
            {t("common.cancel")}
          </Button>
        </div>
      </form>
    </Modal>
  );
}
