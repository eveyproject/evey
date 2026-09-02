import { memo, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { toast } from "@evey/store";
import type { SyncPlayMember } from "./useSyncPlay";

interface SyncPlayBarProps {
  roomId: string;
  roomCode: string | null;
  isHost: boolean;
  members: SyncPlayMember[];
  connected: boolean;
  onLeave: () => void;
  /** v2 "everyone sync to me": pull the whole party to this member's position. */
  onResync?: () => void;
  // Pulls only the caller back to the room's position - nobody else moves.
  onCatchUp?: () => void;
}

export const SyncPlayBar = memo(function SyncPlayBar({ roomId, roomCode, isHost, members, connected, onLeave, onResync, onCatchUp }: SyncPlayBarProps) {
  const { t } = useTranslation();
  const [copied, setCopied] = useState(false);
  const [codeCopied, setCodeCopied] = useState(false);
  const copiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const codeCopiedTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    return () => {
      if (copiedTimerRef.current) clearTimeout(copiedTimerRef.current);
      if (codeCopiedTimerRef.current) clearTimeout(codeCopiedTimerRef.current);
    };
  }, []);

  const shareUrl = `${window.location.origin}${window.location.pathname}?room=${roomId}`;

  function handleCopy() {
    navigator.clipboard.writeText(shareUrl).then(
      () => {
        setCopied(true);
        copiedTimerRef.current = setTimeout(() => setCopied(false), 2000);
      },
      () => toast.error(t("common.clipboardFailed")),
    );
  }

  function handleCopyCode() {
    if (!roomCode) return;
    navigator.clipboard.writeText(roomCode).then(
      () => {
        setCodeCopied(true);
        codeCopiedTimerRef.current = setTimeout(() => setCodeCopied(false), 2000);
      },
      () => toast.error(t("common.clipboardFailed")),
    );
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
      className="flex items-center gap-3 px-4 py-2 text-sm bg-[var(--overlay-bg-strong)] border-t border-[var(--overlay-border-subtle)] text-[var(--overlay-text)]"
    >
      <span
        role="img"
        aria-label={connected ? t("media.syncplay.connected") : t("media.syncplay.connecting")}
        className="w-2 h-2 rounded-full flex-shrink-0"
        style={{ background: connected ? "var(--color-success)" : "var(--color-danger)" }}
        title={connected ? t("media.syncplay.connected") : t("media.syncplay.connecting")}
      />
      <span className="flex-1 truncate">{watchingWith}</span>
      {isHost && roomCode && (
        <button
          onClick={handleCopyCode}
          className="px-3 py-1 rounded text-xs font-bold hover:opacity-80 transition-opacity-fast flex-shrink-0 bg-[var(--overlay-btn-bg)] tracking-[0.15em]"
          title={t("media.syncplay.copyCode")}
          aria-label={t("media.syncplay.copyCode")}
        >
          {codeCopied ? t("common.copied") : roomCode}
        </button>
      )}
      <button
        onClick={handleCopy}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity-fast flex-shrink-0 bg-[var(--overlay-btn-bg)]"
        aria-label={t("media.syncplay.shareLink")}
      >
        {copied ? t("common.copied") : t("media.syncplay.shareLink")}
      </button>
      {onCatchUp && (
        <button
          onClick={onCatchUp}
          className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity-fast flex-shrink-0 bg-[var(--overlay-btn-bg)]"
          title={t("media.syncplay.catchUpHint")}
          aria-label={t("media.syncplay.catchUp")}
        >
          {t("media.syncplay.catchUp")}
        </button>
      )}
      {onResync && (
        <button
          onClick={onResync}
          className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity-fast flex-shrink-0 bg-[var(--overlay-btn-bg)]"
          title={t("media.syncplay.resyncAll", "Sync everyone to my position")}
          aria-label={t("media.syncplay.resyncAll", "Sync everyone to my position")}
        >
          {t("media.syncplay.resyncAll", "Sync everyone")}
        </button>
      )}
      <button
        onClick={onLeave}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity-fast flex-shrink-0 bg-[color-mix(in_srgb,var(--color-danger)_30%,transparent)]"
        aria-label={t("media.syncplay.leave")}
      >
        {t("media.syncplay.leave")}
      </button>
    </div>
  );
});
