import { useEffect, useState, useRef } from "react";
import QRCode from "react-qr-code";
import { useUserServiceInitiateQuickConnect, useUserServiceCheckQuickConnect } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useNavigate } from "@tanstack/react-router";
import { Spinner } from "@/components/ui/Spinner";

const REFRESH_INTERVAL_S = 30;

export function QuickConnect() {
  const [code, setCode] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(true);
  const [secondsLeft, setSecondsLeft] = useState(REFRESH_INTERVAL_S);
  const codeSetAt = useRef<number>(0);
  const { setTokens } = useAuthStore();
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

  const { data: checkData } = useUserServiceCheckQuickConnect(code ?? "", {
    query: {
      enabled: !!code && enabled,
      refetchInterval: 3000,
      refetchIntervalInBackground: true,
    },
  });

  useEffect(() => {
    initiate.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire-once on mount; initiate.mutate identity changes every render
  }, []);

  // Auto-refresh code every 30s
  useEffect(() => {
    const id = setInterval(() => {
      setCode(null);
      initiate.mutate();
    }, REFRESH_INTERVAL_S * 1000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- fire-once on mount; initiate.mutate identity changes every render
  }, []);

  // Countdown ticker
  useEffect(() => {
    const id = setInterval(() => {
      const elapsed = Math.floor((Date.now() - codeSetAt.current) / 1000);
      setSecondsLeft(Math.max(0, REFRESH_INTERVAL_S - elapsed));
    }, 1000);
    return () => clearInterval(id);
  }, []);

  useEffect(() => {
    if (checkData?.authorized && checkData.auth?.accessToken) {
      setEnabled(false);
      setTokens(checkData.auth.accessToken);
      void navigate({ to: "/home" });
    }
  }, [checkData, setTokens, navigate]);

  const qrValue = code ? `${window.location.origin}/quickconnect?code=${code}` : "";

  return (
    <div className="mt-8 flex flex-col items-center gap-3">
      <p className="text-[var(--text-muted)] text-xs uppercase tracking-widest">Quick Connect</p>
      <div
        className="bg-white p-3 rounded-[var(--radius-md)]"
        style={{ width: 140, height: 140 }}
      >
        {code ? (
          <QRCode value={qrValue} size={116} />
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
        Scan with a logged-in device to sign in
      </p>
      {code && (
        <p className="text-[var(--text-muted)] text-xs">
          Expires in {secondsLeft}s
        </p>
      )}
    </div>
  );
}
