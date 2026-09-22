import { useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Button, Spinner } from "../components/ui";
import { navigate } from "../app/router";

const LETTERS = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789".split("");

interface JoinResponse { roomId: string; mediaId: string }

/**
 * Joining a watch-together room by its code, typed on an on-screen grid: a
 * remote has no keyboard, and the code is what the host reads out. The player
 * takes the room from there.
 */
export function SyncplayScreen() {
  const { t } = useTranslation();
  const [code, setCode] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState(false);

  const join = async (candidate: string) => {
    setBusy(true);
    setError(false);
    try {
      const { roomId, mediaId } = await eveyFetch<JoinResponse>({ url: "/api/v1/syncplay/join", method: "POST", data: { code: candidate } });
      navigate({ name: "player", mediaId, roomId });
    } catch {
      setError(true);
      setCode("");
    } finally {
      setBusy(false);
    }
  };

  const type = (ch: string) => {
    const next = (code + ch).slice(0, 6);
    setCode(next);
    if (next.length === 6) void join(next);
  };

  return (
    <div className="screen screen-center">
      <h1 className="title">{t("media.syncplay.joinByCode")}</h1>
      <p className="code">{code.padEnd(6, "·")}</p>
      {error ? <p className="error">{t("media.syncplay.joinError")}</p> : null}
      {busy ? <Spinner /> : null}
      <FocusRoot autoFocus className="letter-grid">
        {LETTERS.map((ch, i) => (
          <Button key={ch} autoFocus={i === 0} onPress={() => type(ch)}>{ch}</Button>
        ))}
      </FocusRoot>
      <FocusRoot className="actions">
        <Button onPress={() => setCode((c) => c.slice(0, -1))}>{t("common.delete")}</Button>
        <Button variant="primary" disabled={code.length === 0} onPress={() => void join(code)}>{t("media.syncplay.join")}</Button>
        <Button onPress={() => navigate({ name: "home" })}>{t("common.back")}</Button>
      </FocusRoot>
    </div>
  );
}
