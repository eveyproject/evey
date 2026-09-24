import { useCallback, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { useUserServiceLogin } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { FocusRoot, useBackHandler } from "../tv/focus";
import { Chip, Pressable, Spinner, TextField } from "../components/ui";
import { clearSavedCredentials, getSavedCredentials, saveCredentials } from "../lib/savedCredentials";
import { refreshTokenStore } from "../lib/session";
import { navigate, replace } from "../app/router";

/** Sign in as the Android television lays it out; the remote lands on Quick Connect. */
export function LoginScreen() {
  const { t } = useTranslation();
  const setTokens = useAuthStore((s) => s.setTokens);
  // Filled from "remember me" when the viewer opted in, as on the phone.
  const [saved] = useState(getSavedCredentials);
  const [username, setUsername] = useState(saved?.username ?? "");
  const [password, setPassword] = useState(saved?.password ?? "");
  const [remember, setRemember] = useState(!!saved);
  const [error, setError] = useState<string | null>(null);
  const changeServer = useCallback(() => replace({ name: "server" }), []);
  useBackHandler(changeServer);

  // One request per press: the field's Enter and the button both submit, and
  // isPending lags a render - the server locks an account after five failures.
  const inFlight = useRef(false);
  const login = useUserServiceLogin({
    mutation: {
      onSettled: () => { inFlight.current = false; },
      onSuccess: async (data) => {
        if (!data.accessToken) return;
        // The refresh token comes in the body (X-Persistent-Session): this
        // client keeps it itself, so the next boot needs no password.
        if (data.refreshToken) await refreshTokenStore.setRefreshToken(data.refreshToken);
        if (remember) saveCredentials(username.trim(), password);
        else clearSavedCredentials();
        setTokens(data.accessToken);
        replace({ name: "profiles" });
      },
      onError: (err) => setError(err instanceof Error && err.message ? err.message : t("auth.login.invalidCredentials")),
    },
  });

  function submit() {
    if (!username.trim() || !password || inFlight.current || login.isPending) return;
    inFlight.current = true;
    setError(null);
    login.mutate({ data: { username: username.trim(), password } });
  }

  return (
    <FocusRoot autoFocus className="auth-page">
      <h1 className="auth-title">{t("auth.login.signIn")}</h1>
      <div className="auth-field"><TextField value={username} onChange={setUsername} placeholder={t("auth.login.username")} autoComplete="username" /></div>
      <div className="auth-field"><TextField value={password} onChange={setPassword} placeholder={t("auth.login.password")} type="password" onSubmit={submit} autoComplete="current-password" /></div>
      <div className="auth-field"><Chip label={t("mobile.auth.rememberMe", "Remember me")} active={remember} onPress={() => setRemember((r) => !r)} /></div>
      <Pressable className="auth-primary" label={t("auth.login.signIn")} onPress={submit}>
        {login.isPending ? <Spinner /> : t("auth.login.signIn")}
      </Pressable>
      {error ? <p className="auth-error"><span className="auth-error-title">{t("auth.login.loginFailed")}</span> {error}</p> : null}
      <div className="auth-divider" />
      <Pressable className="auth-qc" autoFocus label={t("auth.settings.quickConnect.title")} onPress={() => navigate({ name: "quickconnect" })}>
        <span className="auth-qc-title">{t("auth.settings.quickConnect.title")}</span>
        <span className="auth-qc-hint">{t("auth.login.recommendedForTV")}</span>
      </Pressable>
      <Pressable className="auth-link" label={t("mobile.settings.changeServer")} onPress={changeServer}>
        {t("mobile.settings.changeServer")}
      </Pressable>
    </FocusRoot>
  );
}
