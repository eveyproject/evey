import { useState } from "react";
import { useTranslation } from "react-i18next";
import { eveyFetch, isValidRoomCode, ROOM_CODE_ALPHABET, ROOM_CODE_LENGTH } from "@evey/api";
import { FocusRoot } from "../tv/focus";
import { Pressable, Spinner } from "../components/ui";
import { Icon } from "../components/icons";
import { goBack, replace } from "../app/router";

interface JoinResponse { roomId: string; mediaId: string }

const KEYS_PER_ROW = 6;
const KEYPAD_ROWS: (string | null)[][] = (() => {
  const keys: string[] = [...ROOM_CODE_ALPHABET, "del"];
  const rows: (string | null)[][] = [];
  for (let i = 0; i < keys.length; i += KEYS_PER_ROW) {
    const row: (string | null)[] = keys.slice(i, i + KEYS_PER_ROW);
    while (row.length < KEYS_PER_ROW) row.push(null);
    rows.push(row);
  }
  return rows;
})();

/**
 * Joining a watch-together room by the code the host reads out, on the
 * Android television's keypad (the room alphabet only). The player opens the
 * live connection once it mounts with the room.
 */
export function SyncplayScreen() {
  const { t } = useTranslation();
  const [code, setCode] = useState("");
  const [joining, setJoining] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const join = async (candidate: string) => {
    if (!isValidRoomCode(candidate) || joining) return;
    setJoining(true);
    setError(null);
    try {
      const { roomId, mediaId } = await eveyFetch<JoinResponse>({ url: "/api/v1/syncplay/join", method: "POST", data: { code: candidate } });
      replace({ name: "player", mediaId, roomId });
    } catch {
      setError(t("media.syncplay.joinError", "Could not join - check the code."));
      setCode("");
      setJoining(false);
    }
  };

  const press = (key: string) => {
    if (joining) return;
    if (error) setError(null);
    if (key === "del") { setCode((c) => c.slice(0, -1)); return; }
    if (code.length >= ROOM_CODE_LENGTH) return;
    const next = code + key;
    setCode(next);
    if (next.length === ROOM_CODE_LENGTH) void join(next);
  };

  return (
    <FocusRoot autoFocus className="auth-page syncplay-page">
      <h1 className="syncplay-title">{t("media.syncplay.join", "Join a room")}</h1>
      <p className="syncplay-desc">{t("media.syncplay.joinByCode", "Join with code")}</p>
      <div className="pin-dots">
        {Array.from({ length: ROOM_CODE_LENGTH }, (_, i) => (
          <span key={i} className={`pin-dot ${i < code.length ? "filled" : ""} ${error ? "error" : ""}`} />
        ))}
      </div>
      <div className="pin-status">{joining ? <Spinner /> : error ? <span className="pin-error">{error}</span> : null}</div>
      <div className="keypad">
        {KEYPAD_ROWS.map((row, ri) => (
          <div key={ri} className="keypad-row">
            {row.map((key, ki) =>
              key === null ? (
                <span key={ki} className="key" />
              ) : (
                <Pressable key={ki} className="tv-card key surface" autoFocus={ri === 0 && ki === 0} label={key === "del" ? t("common.delete", "Delete") : key} onPress={() => press(key)}>
                  {key === "del" ? <Icon name="backspace-outline" size={24} /> : key}
                </Pressable>
              ),
            )}
          </div>
        ))}
      </div>
      <Pressable className="auth-link accent syncplay-back" onPress={goBack}>{t("common.back", "Back")}</Pressable>
    </FocusRoot>
  );
}
