import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useNavigate } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { Lock } from "lucide-react";
import { formatOpening, useProfileLock } from "@evey/client";
import { useMusicPlayerStore } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { useFocusTrap } from "@/hooks/useFocusTrap";

// Full-screen lock shown while the active kids profile is outside its opening
// hours. The server already refuses everything; this says why, when it opens
// again, and offers the two ways out: a parent's PIN, or another profile.
export function ProfileLockOverlay() {
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const qc = useQueryClient();
  const lock = useProfileLock(() => void qc.invalidateQueries());
  const [pin, setPin] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const panelRef = useRef<HTMLDivElement>(null);
  useFocusTrap(panelRef, lock.closed);

  // Whatever was playing stops with the lock: the next segment would be
  // refused anyway, this just does not leave sound running behind the screen.
  useEffect(() => {
    if (!lock.closed) return;
    document.querySelectorAll("video, audio").forEach((m) => (m as HTMLMediaElement).pause());
    useMusicPlayerStore.getState().pauseExternal();
    setPin("");
    setError(null);
  }, [lock.closed]);

  if (!lock.closed) return null;

  async function submit() {
    if (pin.length !== 4) return;
    setBusy(true);
    const res = await lock.unlock(pin);
    setBusy(false);
    setPin("");
    if (res === null) return;
    setError(
      res === "wrong_pin"
        ? t("auth.profile.locked.wrongPin", "Wrong parent PIN")
        : res === "no_parent_pin"
          ? t("auth.profile.locked.noParentPin", "No parent PIN is set on this account.")
          : res === "rate_limited"
            ? t("auth.profile.locked.rateLimited", "Too many attempts, try again in a minute.")
            : t("auth.profile.locked.failed", "Could not unlock the profile."),
    );
  }

  return (
    <div className="fixed inset-0 z-popover flex items-center justify-center bg-black/80 backdrop-blur-sm p-4" data-testid="profile-lock">
      <div
        ref={panelRef}
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="profile-lock-title"
        className="w-full max-w-sm flex flex-col items-center gap-4 text-center rounded-[var(--radius-lg)] border border-[var(--border)] bg-[var(--surface)] p-6 shadow-xl"
      >
        <Lock size={32} className="text-[var(--accent)]" aria-hidden="true" />
        <h2 id="profile-lock-title" className="text-lg font-semibold text-[var(--text)]">
          {t("auth.profile.locked.title", "This profile is locked right now")}
        </h2>
        <p className="text-sm text-[var(--text-muted)]">
          {lock.nextOpen
            ? t("auth.profile.locked.body", "It opens again {{when}}.", { when: formatOpening(lock.nextOpen, i18n.language) })
            : t("auth.profile.locked.bodyNoNext", "No opening is planned in the coming days.")}
        </p>
        <form
          className="w-full flex flex-col gap-2"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <Input
            label={t("auth.profile.locked.pinLabel", "Parent PIN")}
            type="password"
            inputMode="numeric"
            maxLength={4}
            autoComplete="off"
            value={pin}
            onChange={(e) => setPin(e.target.value.replace(/\D/g, "").slice(0, 4))}
            error={error ?? undefined}
          />
          <p className="text-xs text-[var(--text-muted)]">
            {t("auth.profile.locked.pinHint", "A parent's PIN opens the profile until its next opening.")}
          </p>
          <Button type="submit" loading={busy} disabled={pin.length !== 4}>
            {t("auth.profile.locked.unlock", "Unlock")}
          </Button>
        </form>
        <Button variant="ghost" onClick={() => void navigate({ to: "/profile-select" })}>
          {t("auth.profile.locked.switchProfile", "Switch profile")}
        </Button>
      </div>
    </div>
  );
}
