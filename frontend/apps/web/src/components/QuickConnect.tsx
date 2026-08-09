import { useEffect, useState, useRef } from "react";
import { useTranslation } from "react-i18next";
import QRCode from "react-qr-code";
import { useUserServiceInitiateQuickConnect } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useNavigate } from "@tanstack/react-router";
import { Spinner } from "@/components/ui/Spinner";

const REFRESH_INTERVAL_S = 120;

export function QuickConnect() {
  const [code, setCode] = useState<string | null>(null);
  const [secondsLeft, setSecondsLeft] = useState(REFRESH_INTERVAL_S);
  const codeSetAt = useRef<number>(0);
  const { t } = useTranslation();
  const setTokens = useAuthStore((s) => s.setTokens);
  const navigate = useNavigate();

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

  const initiated = useRef(false);
  useEffect(() => {
    if (initiated.current) return;
    initiated.current = true;
    initiate.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire-once on mount
  }, []);

  // Auto-refresh code every REFRESH_INTERVAL_S
  useEffect(() => {
    const id = setInterval(() => {
      setCode(null);
      initiate.mutate();
    }, REFRESH_INTERVAL_S * 1000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire-once on mount
  }, []);

  // Countdown ticker
  useEffect(() => {
    const id = setInterval(() => {
      const elapsed = Math.floor((Date.now() - codeSetAt.current) / 1000);
      setSecondsLeft(Math.max(0, REFRESH_INTERVAL_S - elapsed));
    }, 1000);
    return () => clearInterval(id);
  }, []);

  // SSE: wait for authorization with reconnect on error
  useEffect(() => {
    if (!code) return;
    let attempt = 0;
    let timer: ReturnType<typeof setTimeout> | null = null;
    let closed = false;
    let es: EventSource | null = null;

    function connectSSE() {
      if (closed) return;
      const url = `/api/v1/auth/quick-connect/${code}/events`;
      es = new EventSource(url);
      // A successful (re)connection resets the retry budget; otherwise 3 transient
      // drops across the code's whole 120s window exhaust it and we stop listening
      // for good, leaving the user stuck on the QR after authorizing on their phone.
      es.onopen = () => { attempt = 0; };
      es.onmessage = (e) => {
        try {
          const data = JSON.parse(e.data) as { event: string; accessToken?: string };
          if (data.event === "authorized" && data.accessToken) {
            setTokens(data.accessToken);
            es?.close();
            void navigate({ to: "/profile-select" });
          } else if (data.event === "expired" || data.event === "error") {
            es?.close();
          }
        } catch (err) { console.warn("quickconnect sse parse failed", err); }
      };
      es.onerror = () => {
        es?.close();
        if (closed) return;
        if (attempt < 3) {
          const delay = 1000 * Math.pow(2, attempt); // 1s, 2s, 4s
          attempt += 1;
          timer = setTimeout(connectSSE, delay);
        }
      };
    }
    connectSSE();

    return () => {
      closed = true;
      if (timer) clearTimeout(timer);
      es?.close();
    };
  }, [code, setTokens, navigate]);

  const qrValue = code ? `${window.location.origin}/quickconnect?code=${code}` : "";

  return (
    <div className="mt-8 flex flex-col items-center gap-3">
      <p className="text-[var(--text-muted)] text-xs uppercase tracking-widest">{t("auth.settings.quickConnect.scanLabel")}</p>
      <div
        className="bg-white p-3 rounded-[var(--radius-md)]"
        style={{ width: 140, height: 140 }}
      >
        {code ? (
          <QRCode value={qrValue} size={116} />
        ) : initiate.isError ? (
          // The code request runs on mount: when it fails there is no gesture
          // to repeat, so a bare spinner spun for ever with no way out.
          <div className="w-full h-full flex items-center justify-center">
            <button
              type="button"
              onClick={() => initiate.mutate()}
              data-testid="quick-connect-retry"
              className="text-xs font-medium underline text-black"
            >
              {t("common.retry")}
            </button>
          </div>
        ) : (
          <div className="w-full h-full flex items-center justify-center">
            <Spinner size="sm" />
          </div>
        )}
      </div>
      {code ? (
        <p className="font-mono text-lg tracking-widest text-[var(--text)]">{code.slice(-6).toUpperCase()}</p>
      ) : null}
      <p className="text-[var(--text-muted)] text-xs text-center max-w-[200px]">
        {t("auth.settings.quickConnect.scanInstruction")}
      </p>
      {code && (
        <p className="text-[var(--text-muted)] text-xs">
          {t("auth.settings.quickConnect.expiresIn", { seconds: secondsLeft })}
        </p>
      )}
    </div>
  );
}
