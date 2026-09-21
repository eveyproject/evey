import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { useUserServiceGetCurrentUser, useUserServiceListProfiles, userServiceRefreshToken, type V1Profile } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button, Spinner } from "../components/ui";
import { replace } from "../app/router";

// A profile is a server-side grant, not a client flag: switching means a
// refresh with the profile id, so the token the player sends carries it.
// A profile behind a PIN refuses the switch; the PIN prompt comes with a
// later milestone.
export function ProfileSelectScreen() {
  const { t } = useTranslation();
  const qc = useQueryClient();
  const { data: user } = useUserServiceGetCurrentUser();
  const { data, isLoading } = useUserServiceListProfiles(user?.id ?? "", { query: { enabled: !!user?.id } });
  const [busy, setBusy] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const profiles = data?.profiles ?? [];

  async function choose(p: V1Profile) {
    if (!p.id || busy) return;
    setBusy(p.id);
    setError(null);
    try {
      const resp = await userServiceRefreshToken({ profileId: p.id });
      if (resp.accessToken) useAuthStore.getState().setTokens(resp.accessToken);
      useAuthStore.getState().setProfile(p.id);
      await qc.invalidateQueries();
      replace({ name: "home" });
    } catch {
      setError(t("common.error", "Something went wrong."));
    } finally {
      setBusy(null);
    }
  }

  return (
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">{t("profile.select.title", "Who is watching?")}</h1>
      {isLoading ? <Spinner /> : null}
      <div className="actions" style={{ flexWrap: "wrap", justifyContent: "center" }}>
        {profiles.map((p, i) => (
          <Button key={p.id} autoFocus={i === 0} variant={p.isDefault ? "primary" : "default"} onPress={() => void choose(p)} disabled={!!busy}>
            {p.name}
          </Button>
        ))}
      </div>
      {error ? <p className="error">{error}</p> : null}
    </FocusRoot>
  );
}
