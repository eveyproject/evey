import { useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, userServiceRefreshToken, type V1Profile } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Pressable, Spinner } from "../components/ui";
import { ScreenState } from "../components/ScreenState";
import { replace } from "../app/router";
import { refreshTokenStore } from "../lib/session";
import { ParentalPINModal } from "../components/ParentalPINModal";
import { walksThroughProfilePicker } from "@evey/client";
import { avatarSrc } from "../lib/media";

// A profile is a server-side grant, not a client flag: switching means a
// refresh with the profile id, so the token the player sends carries it.
// A profile behind a PIN refuses the switch with a 403 until the PIN's
// short-lived proof rides along - the prompt asks for it, the server checks.
export function ProfileSelectScreen({ pick }: { pick?: boolean }) {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data, isLoading, isError, refetch } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);
  const [pinError, setPinError] = useState<string | null>(null);
  const profiles = data?.profiles ?? [];
  const activeProfileId = useAuthStore((s) => s.profileId);

  async function choose(p: V1Profile, pinToken?: string) {
    if (!p.id || busy) return;
    setBusy(p.id);
    setError(null);
    try {
      // No cookie rides along (see setCredentialsMode): the refresh token goes
      // in the body, and the rotated one that comes back replaces it - the
      // server single-uses them, so keeping the old one would end the
      // session at the next refresh.
      const refreshToken = (await refreshTokenStore.getRefreshToken()) ?? undefined;
      const resp = await userServiceRefreshToken({ profileId: p.id, refreshToken, pinToken });
      if (resp.refreshToken) await refreshTokenStore.setRefreshToken(resp.refreshToken);
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      useAuthStore.getState().setProfile(p.id);
      await qc.invalidateQueries();
      replace({ name: "home" });
    } catch (err) {
      // 403 without a proof: the profile is behind a PIN, ask for it. With
      // a proof the server still refused: the PIN was a stranger's.
      const status = (err as { httpStatus?: number })?.httpStatus;
      if (status === 403) {
        // Refused with a proof too: the digits were not the gate's (a
        // PIN-less target answers "valid" to anything, and the server
        // rightly ignores that proof). Ask again rather than give up.
        setPinError(pinToken ? t("common.parental.pinIncorrect", "Incorrect PIN") : null);
        setPinTarget(p);
      } else setError(t("common.error", "Something went wrong."));
    } finally {
      setBusy(null);
    }
  }

  // Right after sign-in, a household with one profile or no kids profile has
  // nothing to protect: walk straight through, as the Android television
  // does. A viewer who asked to switch (pick) always gets the picker.
  const walkedThrough = useRef(false);
  const hasKids = profiles.some((p) => p.isKids === true);
  const walkThrough = walksThroughProfilePicker({ asked: !!pick, profileCount: profiles.length, hasKidsProfile: hasKids });
  useEffect(() => {
    if (isLoading || profiles.length === 0 || !walkThrough || walkedThrough.current) return;
    walkedThrough.current = true;
    void choose(profiles.find((p) => p.isDefault) ?? profiles[0]);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- once, when the list lands
  }, [isLoading, profiles.length, walkThrough]);

  if (isError || (!isLoading && user && profiles.length === 0)) return <ScreenState error onRetry={() => void refetch()} />;
  if (isLoading || !user || (walkThrough && !pinTarget && !error)) return <ScreenState loading />;

  return (
    <FocusRoot autoFocus className="auth-page">
      <h1 className="auth-title profiles-title">{t("mobile.profileSelect.title", "Who's watching?")}</h1>
      <div className="profile-cards">
        {profiles.map((p, i) => (
          <Pressable key={p.id} className="tv-card profile-card" autoFocus={i === 0} label={p.name} onPress={() => void choose(p)}>
            <span className="profile-face">{p.avatarPath && p.id ? <img src={avatarSrc(p.id)} alt="" /> : (p.name ?? "?")[0]?.toUpperCase() ?? "?"}</span>
            <span className="profile-name-row">
              <span className="profile-card-name">{p.name}</span>
              {p.isKids ? <span className="kids-badge">{t("auth.profile.select.kidsBadge", "Kids")}</span> : null}
            </span>
            {p.id === activeProfileId ? <span className="profile-active">{t("auth.profile.select.active", "Active")}</span> : null}
            {busy === p.id ? <Spinner /> : null}
          </Pressable>
        ))}
      </div>
      {error ? <p className="auth-error">{error}</p> : null}
      {pinTarget ? (
        <ParentalPINModal
          profileId={pinTarget.id!}
          fallbackProfileId={useAuthStore.getState().profileId ?? undefined}
          initialError={pinError}
          onVerified={(token) => { const target = pinTarget; setPinTarget(null); void choose(target, token); }}
          onClose={() => setPinTarget(null)}
        />
      ) : null}
    </FocusRoot>
  );
}
