import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useServerListStore } from "@evey/store";
import { FocusRoot } from "../tv/focus";
import { Button, TextField, Spinner } from "../components/ui";
import { probeServer, selectServer } from "../lib/session";
import { replace } from "../app/router";

export function ServerConnectScreen() {
  const { t } = useTranslation();
  // Select the record itself: a selector returning a fresh array every call
  // makes zustand re-render without end.
  const serverMap = useServerListStore((s) => s.servers);
  const servers = useMemo(() => Object.values(serverMap), [serverMap]);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);

  async function connect(candidate: string) {
    const trimmed = candidate.trim();
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    const withScheme = /^https?:\/\//i.test(trimmed) ? trimmed : `http://${trimmed}`;
    const { probe, health } = await probeServer(withScheme);
    setBusy(false);
    if (probe !== "ok") {
      setError(probe === "not-evey" ? t("mobile.serverConnect.notEvey", "This address answers, but it is not an Evey server.") : t("mobile.serverConnect.unreachable", "No Evey server answers at this address."));
      return;
    }
    selectServer(withScheme, health?.name);
    replace({ name: "login" });
  }

  return (
    <FocusRoot autoFocus className="screen screen-center">
      <h1 className="title">Evey TV</h1>
      <p className="subtitle">{t("mobile.serverConnect.subtitle", "Enter the address of your Evey server.")}</p>
      <div className="stack">
        <TextField value={url} onChange={setUrl} placeholder="https://evey.example.com" type="url" autoFocus onSubmit={() => void connect(url)} />
        <Button variant="primary" onPress={() => void connect(url)} disabled={busy}>
          {t("mobile.serverConnect.connect", "Connect")}
        </Button>
        {busy ? <Spinner /> : null}
        {error ? <p className="error">{error}</p> : null}
      </div>
      {servers.length > 0 ? (
        <div className="stack" style={{ marginTop: "2vw" }}>
          <p className="muted">{t("mobile.serverConnect.savedServers", "Saved servers")}</p>
          {servers.map((s) => (
            <Button key={s.id} onPress={() => void connect(s.url)}>
              {s.label || s.serverName || s.url}
            </Button>
          ))}
        </div>
      ) : null}
    </FocusRoot>
  );
}
