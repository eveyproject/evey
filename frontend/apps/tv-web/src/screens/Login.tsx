import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useUserServiceLogin } from "@evey/api";
import { useAuthStore, useServerListStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button, TextField } from "../components/ui";
import { refreshTokenStore } from "../lib/session";
import { navigate, replace } from "../app/router";

export function LoginScreen() {
  const { t } = useTranslation();
  const server = useServerListStore((s) => s.getActiveServer());
  const setTokens = useAuthStore((s) => s.setTokens);
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [error, setError] = useState<string | null>(null);

  const login = useUserServiceLogin({
    mutation: {
      onSuccess: async (data) => {
        if (!data.accessToken) return;
        // The refresh token comes in the body (X-Persistent-Session): this
        // client keeps it itself, so the next boot needs no password.
        if (data.refreshToken) await refreshTokenStore.setRefreshToken(data.refreshToken);
        setTokens(data.accessToken);
        replace({ name: "profiles" });
      },
      onError: () => setError(t("auth.login.loginFailed", "Sign-in failed. Check your username and password.")),
    },
  });

  function submit() {
    if (!username.trim() || !password || login.isPending) return;
    setError(null);
    login.mutate({ data: { username: username.trim(), password } });
  }

  return (
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">{t("auth.login.signIn", "Sign in")}</h1>
      <p className="subtitle">{server?.label || server?.serverName || server?.url}</p>
      <div className="stack">
        <TextField value={username} onChange={setUsername} placeholder={t("auth.login.username", "Username")} autoFocus onSubmit={submit} />
        <TextField value={password} onChange={setPassword} placeholder={t("auth.login.password", "Password")} type="password" onSubmit={submit} />
        <Button variant="primary" onPress={submit} disabled={login.isPending}>
          {t("auth.login.signIn", "Sign in")}
        </Button>
        {error ? <p className="error">{error}</p> : null}
      </div>
      <div className="actions">
        <Button onPress={() => navigate({ name: "quickconnect" })}>
          {t("auth.settings.quickConnect.title", "Quick Connect")}
        </Button>
        <Button variant="ghost" onPress={() => replace({ name: "server" })}>
          {t("mobile.settings.changeServer", "Change server")}
        </Button>
      </div>
    </FocusRoot>
  );
}
