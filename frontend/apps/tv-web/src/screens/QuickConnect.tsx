import { useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import QRCode from "react-qr-code";
import { useUserServiceInitiateQuickConnect, userServiceCheckQuickConnect } from "@evey/api";
import { useAuthStore, useServerListStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button, Spinner } from "../components/ui";
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

  const initiate = useUserServiceInitiateQuickConnect({
    mutation: {
      onSuccess: (data) => {
        if (data.code) {
          setCode(data.code);
          codeSetAt.current = Date.now();
          setSecondsLeft(REFRESH_INTERVAL_S);
        }
      },
    },
  });
  const initiateRef = useRef(initiate);
  initiateRef.current = initiate;
  const doInitiate = useCallback(() => initiateRef.current.mutate(), []);

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
        const resp = await userServiceCheckQuickConnect(code);
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
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">{t("auth.settings.quickConnect.title", "Quick Connect")}</h1>
      <p className="subtitle">{t("auth.settings.quickConnect.scanInstruction", "Scan the code or enter it on a device where you are signed in.")}</p>
      {qrValue ? (
        <div className="qr">
          <QRCode value={qrValue} size={220} />
        </div>
      ) : initiate.isError ? (
        <Button onPress={doInitiate}>{t("common.retry", "Retry")}</Button>
      ) : (
        <Spinner />
      )}
      {displayCode ? <div className="code">{displayCode}</div> : null}
      {code ? <p className="muted">{t("auth.settings.quickConnect.expiresIn", "Expires in {{seconds}} s", { seconds: secondsLeft })}</p> : null}
      <Button autoFocus onPress={goBack}>{t("common.back", "Back")}</Button>
    </FocusRoot>
  );
}
