import { useEffect, useState } from "react";
import QRCode from "react-qr-code";
import { useUserServiceInitiateQuickConnect, useUserServiceCheckQuickConnect } from "@evey/api";
import { useAuthStore } from "@evey/store";
import { useNavigate } from "@tanstack/react-router";
import { Spinner } from "@/components/ui/Spinner";

export function QuickConnect() {
  const [code, setCode] = useState<string | null>(null);
  const [enabled, setEnabled] = useState(true);
  const { setTokens } = useAuthStore();
  const navigate = useNavigate();

  const initiate = useUserServiceInitiateQuickConnect({
    mutation: {
      onSuccess: (data) => {
        if (data.code) setCode(data.code);
      },
    },
  });

  const { data: checkData } = useUserServiceCheckQuickConnect(code ?? "", {
    query: {
      enabled: !!code && enabled,
      refetchInterval: 3000,
    },
  });

  useEffect(() => {
    initiate.mutate();
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Auto-refresh code every 30s
  useEffect(() => {
    const id = setInterval(() => {
      setCode(null);
      initiate.mutate();
    }, 30_000);
    return () => clearInterval(id);
    // eslint-disable-next-line react-hooks/exhaustive-deps
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
    </div>
  );
}
