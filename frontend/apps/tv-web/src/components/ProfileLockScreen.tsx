import { useEffect, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { formatOpening, useProfileLock } from "@evey/client";
import { useAuthStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { replace } from "../app/router";
import { useMusicQueueStore } from "../music/queue";
import { Pressable } from "./ui";
import { ParentalPINModal } from "./ParentalPINModal";

// Full-screen lock while the active kids profile is outside its opening hours,
// driven by the remote like the Android television's: a parent's PIN on the
// keypad, or another profile.
export function ProfileLockScreen() {
  const { t, i18n } = useTranslation();
  const qc = useQueryClient();
  const profileId = useAuthStore((s) => s.profileId) ?? "";
  const lock = useProfileLock(() => void qc.invalidateQueries());
  const [pinOpen, setPinOpen] = useState(false);

  // Whatever plays stops with the lock: the next segment would be refused.
  useEffect(() => {
    if (!lock.closed) {
      setPinOpen(false);
      return;
    }
    document.querySelectorAll("video, audio").forEach((m) => (m as HTMLMediaElement).pause());
    useMusicQueueStore.setState({ isPlaying: false });
  }, [lock.closed]);

  if (!lock.closed) return null;

  async function submitPin(pin: string): Promise<string | null> {
    const res = await lock.unlock(pin);
    if (res === null) return null;
    return res === "wrong_pin"
      ? t("auth.profile.locked.wrongPin", "Wrong parent PIN")
      : res === "no_parent_pin"
        ? t("auth.profile.locked.noParentPin", "No parent PIN is set on this account.")
        : res === "rate_limited"
          ? t("auth.profile.locked.rateLimited", "Too many attempts, try again in a minute.")
          : t("auth.profile.locked.failed", "Could not unlock the profile.");
  }

  if (pinOpen) {
    return (
      <ParentalPINModal
        profileId={profileId}
        title={t("auth.profile.locked.title", "This profile is locked right now")}
        description={t("auth.profile.locked.pinHint", "A parent's PIN opens the profile until its next opening.")}
        submitPin={submitPin}
        onVerified={() => setPinOpen(false)}
        onClose={() => setPinOpen(false)}
      />
    );
  }

  return (
    <div className="modal-backdrop profile-lock" role="alertdialog" aria-labelledby="profile-lock-title">
      <FocusRoot autoFocus boundary className="pin-dialog">
        <h2 id="profile-lock-title" className="pin-title">{t("auth.profile.locked.title", "This profile is locked right now")}</h2>
        <p className="pin-desc">
          {lock.nextOpen
            ? t("auth.profile.locked.body", "It opens again {{when}}.", { when: formatOpening(lock.nextOpen, i18n.language) })
            : t("auth.profile.locked.bodyNoNext", "No opening is planned in the coming days.")}
        </p>
        <Pressable className="tv-card pin-cancel" autoFocus onPress={() => setPinOpen(true)}>
          {t("auth.profile.locked.pinLabel", "Parent PIN")}
        </Pressable>
        <Pressable className="tv-card pin-cancel" onPress={() => replace({ name: "profiles", pick: true })}>
          {t("auth.profile.locked.switchProfile", "Switch profile")}
        </Pressable>
        <p className="pin-desc">{t("auth.profile.locked.pinHint", "A parent's PIN opens the profile until its next opening.")}</p>
      </FocusRoot>
    </div>
  );
}
