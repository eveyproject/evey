import { memo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { SyncPlayMember } from "./useSyncPlay";

interface SyncPlayBarProps {
  roomId: string;
  roomCode: string | null;
  isHost: boolean;
  members: SyncPlayMember[];
  connected: boolean;
  onLeave: () => void;
}

export const SyncPlayBar = memo(function SyncPlayBar({ roomId, roomCode, isHost, members, connected, onLeave }: SyncPlayBarProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const [codeCopied, setCodeCopied] = useState(false);

  const shareUrl = `${window.location.origin}${window.location.pathname}?room=${roomId}`;

  function handleCopy() {
    void navigator.clipboard.writeText(shareUrl).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  function handleCopyCode() {
    if (!roomCode) return;
    void navigator.clipboard.writeText(roomCode).then(() => {
      setCodeCopied(true);
      setTimeout(() => setCodeCopied(false), 2000);
    });
  }

  const displayNames = members.slice(0, 3).map((m) => m.username);
  const extra = members.length > 3 ? members.length - 3 : 0;
  const watchingWith = displayNames.length > 0
    ? extra > 0
      ? t("media.syncplay.watchingWithMore", { names: displayNames.join(", "), count: extra })
      : t("media.syncplay.watchingWith", { names: displayNames.join(", ") })
    : t("media.syncplay.watchTogetherRoom");

  return (
    <div
      className="flex items-center gap-3 px-4 py-2 text-sm"
      style={{ background: "var(--overlay-bg-strong)", borderTop: "1px solid var(--overlay-border-subtle)", color: "var(--overlay-text)" }}
    >
      <span
        className="w-2 h-2 rounded-full flex-shrink-0"
        style={{ background: connected ? "var(--color-success)" : "var(--color-danger)" }}
        title={connected ? t("media.syncplay.connected") : t("media.syncplay.connecting")}
      />
      <span className="flex-1 truncate">{watchingWith}</span>
      {isHost && roomCode && (
        <button
          onClick={handleCopyCode}
          className="px-3 py-1 rounded text-xs font-bold tracking-widest hover:opacity-80 transition-opacity flex-shrink-0"
          style={{ background: "var(--overlay-btn-bg)", letterSpacing: "0.15em" }}
          title={t("media.syncplay.copyCode")}
        >
          {codeCopied ? t("common.copied") : roomCode}
        </button>
      )}
      <button
        onClick={handleCopy}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity flex-shrink-0"
        style={{ background: "var(--overlay-btn-bg)" }}
      >
        {copied ? t("common.copied") : t("media.syncplay.shareLink")}
      </button>
      <button
        onClick={onLeave}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity flex-shrink-0"
        style={{ background: "color-mix(in srgb, var(--color-danger) 30%, transparent)" }}
      >
        {t("media.syncplay.leave")}
      </button>
    </div>
  );
});
