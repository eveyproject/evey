import { useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import { useAuthStore, useServerListStore } from "@evey/store";
import { FocusRoot, useBackHandler } from "../tv/focus";
import { Pressable, Spinner, TextField } from "../components/ui";
import { probeServer, selectServer } from "../lib/session";
import { navigate, replace } from "../app/router";

type Saved = { id: string; url: string; label?: string; serverName?: string };

function displayName(s: Saved): string {
  return s.label || s.serverName || s.url;
}

function Rename({ server, onClose }: { server: Saved; onClose: () => void }) {
  const { t } = useTranslation();
  const renameServer = useServerListStore((s) => s.renameServer);
  const [value, setValue] = useState(server.label ?? "");
  useBackHandler(onClose);
  const save = () => { renameServer(server.id, value.trim()); onClose(); };
  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus boundary className="confirm-box">
        <h2 className="confirm-title">{t("mobile.servers.rename", "Rename")}</h2>
        <div className="auth-field wide"><TextField value={value} onChange={setValue} placeholder={t("mobile.servers.renamePlaceholder", "Server name")} autoFocus onSubmit={save} /></div>
        <div className="confirm-actions">
          <Pressable className="confirm-btn" onPress={onClose}>{t("common.cancel")}</Pressable>
          <Pressable className="confirm-btn primary" onPress={save}>{t("common.save", "Save")}</Pressable>
        </div>
      </FocusRoot>
    </div>
  );
}

function ConfirmDelete({ server, onClose }: { server: Saved; onClose: () => void }) {
  const { t } = useTranslation();
  const removeServer = useServerListStore((s) => s.removeServer);
  useBackHandler(onClose);
  return (
    <div className="modal-backdrop">
      <FocusRoot autoFocus boundary className="confirm-box">
        <h2 className="confirm-title">{t("mobile.servers.deleteConfirmTitle", "Delete server?")}</h2>
        <p className="confirm-body">{t("mobile.servers.deleteConfirmMessage", { name: displayName(server), defaultValue: `${displayName(server)} will be removed.` })}</p>
        <div className="confirm-actions">
          <Pressable className="confirm-btn" autoFocus onPress={onClose}>{t("common.cancel")}</Pressable>
          <Pressable className="confirm-btn danger" onPress={() => { removeServer(server.id); onClose(); }}>{t("mobile.servers.delete", "Delete")}</Pressable>
        </div>
      </FocusRoot>
    </div>
  );
}

/** The saved servers: OK switches, Rename and Delete sit beside each one. */
export function SavedServers({ onPick, onError }: { onPick: (s: Saved) => void; onError: (message: string) => void }) {
  const { t } = useTranslation();
  const serverMap = useServerListStore((s) => s.servers);
  const activeId = useServerListStore((s) => s.activeServerId);
  const servers = useMemo(() => (Object.values(serverMap) as (Saved & { lastUsedAt?: number })[]).sort((a, b) => (b.lastUsedAt ?? 0) - (a.lastUsedAt ?? 0)), [serverMap]);
  const [renaming, setRenaming] = useState<Saved | null>(null);
  const [deleting, setDeleting] = useState<Saved | null>(null);
  return (
    <>
      <FocusRoot className="server-list">
        {servers.map((s, i) => {
          const active = s.id === activeId;
          const onlyOne = servers.length === 1 && active;
          return (
            <div key={s.id} className={`server-row ${i > 0 ? "sep" : ""}`}>
              <Pressable className="server-main" autoFocus={i === 0 && active} label={displayName(s)} onPress={() => onPick(s)}>
                <span className="server-names">
                  <span className={`server-label ${active ? "accent" : ""}`}>{displayName(s)}</span>
                  {displayName(s) !== s.url ? <span className="server-sub">{s.url}</span> : null}
                </span>
                {active ? <span className="server-badge">{t("mobile.servers.active", "Active")}</span> : null}
              </Pressable>
              <div className="server-actions">
                <Pressable className="server-action" onPress={() => setRenaming(s)}>{t("mobile.servers.rename", "Rename")}</Pressable>
                <Pressable className={`server-action danger ${onlyOne ? "off" : ""}`} onPress={() => { if (onlyOne) onError(t("mobile.servers.cantDeleteOnlyServer", "Can't delete the only saved server")); else setDeleting(s); }}>
                  {t("mobile.servers.delete", "Delete")}
                </Pressable>
              </div>
            </div>
          );
        })}
      </FocusRoot>
      {renaming ? <Rename server={renaming} onClose={() => setRenaming(null)} /> : null}
      {deleting ? <ConfirmDelete server={deleting} onClose={() => setDeleting(null)} /> : null}
    </>
  );
}

/** The server screen as the Android television lays it out, less the local
 * network scan: a browser cannot probe every address of the subnet (CORS). */
export function ServerConnectScreen() {
  const { t } = useTranslation();
  const serverMap = useServerListStore((s) => s.servers);
  const signedIn = !!useAuthStore((s) => s.accessToken);
  const servers = Object.values(serverMap);
  const [url, setUrl] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  async function connect() {
    const trimmed = url.trim().replace(/\/+$/, "");
    if (!trimmed || busy) return;
    setBusy(true);
    setError(null);
    const withScheme = /^https?:\/\//i.test(trimmed) ? trimmed : `http://${trimmed}`;
    const { probe, health } = await probeServer(withScheme);
    setBusy(false);
    if (probe !== "ok") {
      setError(probe === "not-evey" ? t("mobile.serverConnect.notEvey", { url: withScheme, defaultValue: "{{url}} answered, but it is not an Evey server." }) : t("mobile.serverConnect.unreachable", "Unable to reach server"));
      return;
    }
    selectServer(withScheme, health?.name);
    replace({ name: "login" });
  }

  const pick = (s: Saved) => {
    selectServer(s.url);
    replace({ name: "login" });
  };

  return (
    <FocusRoot autoFocus className="auth-page server-page">
      <div className="server-brand">Evey</div>
      <h1 className="server-heading">{t("mobile.serverConnect.title", "Connect to your server")}</h1>

      {servers.length > 0 ? <SavedServers onPick={pick} onError={setError} /> : null}

      <p className={`server-section ${servers.length > 0 ? "spaced" : ""}`}>
        {servers.length > 0 ? t("mobile.servers.addAnother", "Add another server") : t("mobile.serverConnect.title", "Connect to your server")}
      </p>
      <FocusRoot className="server-form">
        <div className="server-field"><TextField value={url} onChange={setUrl} placeholder="https://your-server.com" type="url" autoFocus={servers.length === 0} onSubmit={() => void connect()} /></div>
        <Pressable className="server-connect" label={t("mobile.serverConnect.connect", "Connect")} onPress={() => void connect()}>
          {busy ? <Spinner /> : t("mobile.serverConnect.connect", "Connect")}
        </Pressable>
        {error ? <p className="auth-error">{error}</p> : null}
        {servers.length > 0 && signedIn ? (
          <Pressable className="server-manage" onPress={() => navigate({ name: "servers" })}>{t("mobile.servers.manageTitle", "Manage Servers")}</Pressable>
        ) : null}
      </FocusRoot>

    </FocusRoot>
  );
}
