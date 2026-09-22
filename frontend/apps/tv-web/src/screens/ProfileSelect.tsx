import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, userServiceRefreshToken, type V1Profile } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button, Spinner } from "../components/ui";
import { replace } from "../app/router";
import { refreshTokenStore } from "../lib/session";
import { PinPrompt } from "../components/PinPrompt";

// A profile is a server-side grant, not a client flag: switching means a
// refresh with the profile id, so the token the player sends carries it.
// A profile behind a PIN refuses the switch with a 403 until the PIN's
// short-lived proof rides along - the prompt asks for it, the server checks.
export function ProfileSelectScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data, isLoading } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pinTarget, setPinTarget] = useState<V1Profile | null>(null);
  const [pinError, setPinError] = useState<string | null>(null);
  const profiles = data?.profiles ?? [];

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

  return (
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">{t("auth.profile.select.title", "Who is watching?")}</h1>
      {isLoading ? <Spinner /> : null}
      <div className="actions" style={{ flexWrap: "wrap", justifyContent: "center" }}>
        {profiles.map((p, i) => (
          <Button key={p.id} autoFocus={i === 0} variant={p.isDefault ? "primary" : "default"} onPress={() => void choose(p)} disabled={!!busy}>
            {p.name}
          </Button>
        ))}
      </div>
      {error ? <p className="error">{error}</p> : null}
      {pinTarget ? (
        <PinPrompt
          profileId={pinTarget.id!}
          profileName={pinTarget.name ?? ""}
          fallbackProfileId={useAuthStore.getState().profileId ?? undefined}
          initialError={pinError}
          onVerified={(token) => { const target = pinTarget; setPinTarget(null); void choose(target, token); }}
          onCancel={() => setPinTarget(null)}
        />
      ) : null}
    </FocusRoot>
  );
}
