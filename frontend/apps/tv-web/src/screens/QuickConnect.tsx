import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import QRCode from "react-qr-code";
import { useUserServiceInitiateQuickConnect, userServiceCheckQuickConnect } from "@evey/api";
import { useAuthStore, useServerListStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Pressable, Spinner } from "../components/ui";
import { refreshTokenStore } from "../lib/session";
import { goBack, replace } from "../app/router";

const REFRESH_INTERVAL_S = 120;

// The television shows a code, the viewer approves it from a phone or a
// computer that is already signed in, and never types a password with the
// remote. Polling rather than Server-Sent Events: EventSource exists in a
// TV browser, but a poll every 3 s on a 2-minute code is the same trade-off
// the phone and the Android TV app made, and one code path is worth more
// than a few requests.
export function QuickConnectScreen() {
  const { t } = useTranslation();
  const serverUrl = useServerListStore((s) => s.getActiveServer()?.url);
  const setTokens = useAuthStore((s) => s.setTokens);
  const [code, setCode] = useState<string | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(REFRESH_INTERVAL_S);
  const codeSetAt = useRef(0);
  // The secret comes back with the code and never leaves this device: the
  // QR carries the code, the poll carries both, so a phone that scanned the
  // screen cannot collect the tokens in our place.
  const secretRef = useRef<string | null>(null);

  const initiate = useUserServiceInitiateQuickConnect({
    mutation: {
      onSuccess: (data) => {
        if (data.code) {
          setCode(data.code);
          secretRef.current = data.secret ?? null;
          codeSetAt.current = Date.now();
          setSecondsLeft(REFRESH_INTERVAL_S);
        }
      },
    },
  });
  const initiateRef = useRef(initiate);
  initiateRef.current = initiate;
  const doInitiate = useCallback(() => initiateRef.current.mutate({ params: { secretAware: true } }), []);

  useEffect(() => {
    doInitiate();
    const id = setInterval(() => {
      setCode(null);
      doInitiate();
    }, REFRESH_INTERVAL_S * 1000);
    return () => clearInterval(id);
  }, [doInitiate]);

  useEffect(() => {
    const id = setInterval(() => {
      const elapsed = Math.floor((Date.now() - codeSetAt.current) / 1000);
      setSecondsLeft(Math.max(0, REFRESH_INTERVAL_S - elapsed));
    }, 1000);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    if (!code) return;
    let cancelled = false;
    async function tick() {
      if (cancelled || !code) return;
      try {
        const resp = await userServiceCheckQuickConnect(code, secretRef.current ? { secret: secretRef.current } : undefined);
        if (cancelled) return;
        if (resp.authorized && resp.auth?.accessToken) {
          if (resp.auth.refreshToken) await refreshTokenStore.setRefreshToken(resp.auth.refreshToken);
          setTokens(resp.auth.accessToken);
          replace({ name: "profiles" });
        }
      } catch {
        // Network blip - keep polling, the next tick retries.
      }
    }
    const id = setInterval(() => void tick(), 3000);
    void tick();
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [code, setTokens]);

  const displayCode = code ? code.slice(-6).toUpperCase() : null;
  const qrValue = code && serverUrl ? `${serverUrl}/quickconnect?code=${code}` : null;

  return (
    <FocusRoot autoFocus className="auth-page qc-page">
      <h1 className="qc-title">{t("auth.settings.quickConnect.title")}</h1>
      <p className="qc-instruction">{t("auth.settings.quickConnect.scanInstruction")}</p>
      <div className="qc-box">
        {qrValue ? (
          <div className="qc-qr"><QRCode value={qrValue} size={170} style={{ width: "17rem", height: "17rem" }} /></div>
        ) : initiate.isError ? (
          <Pressable className="tv-card qc-retry" onPress={doInitiate}>{t("common.retry")}</Pressable>
        ) : (
          <div className="qc-wait"><Spinner /></div>
        )}
        {displayCode ? <div className="qc-code">{displayCode}</div> : null}
      </div>
      {code ? <p className="qc-expires">{t("auth.settings.quickConnect.expiresIn", { seconds: secondsLeft })}</p> : null}
      <Pressable className="auth-link accent" autoFocus label={t("auth.forgotPassword.backToLogin")} onPress={goBack}>
        {t("common.back")}
      </Pressable>
    </FocusRoot>
  );
}
