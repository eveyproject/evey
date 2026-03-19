import { useState } from "react";
import { useTranslation } from "react-i18next";
import type { SyncPlayMember } from "./useSyncPlay";

interface SyncPlayBarProps {
  roomId: string;
  members: SyncPlayMember[];
  connected: boolean;
  onLeave: () => void;
}

export function SyncPlayBar({ roomId, members, connected, onLeave }: SyncPlayBarProps) {
  const { t } = useTranslation("media");
  const { t: tc } = useTranslation("common");
  const [copied, setCopied] = useState(false);

  const shareUrl = `${window.location.origin}${window.location.pathname}?room=${roomId}`;

  function handleCopy() {
    void navigator.clipboard.writeText(shareUrl).then(() => {
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    });
  }

  const displayNames = members.slice(0, 3).map((m) => m.username);
  const extra = members.length > 3 ? members.length - 3 : 0;
  const watchingWith = displayNames.length > 0
    ? extra > 0
      ? t("syncplay.watchingWithMore", { names: displayNames.join(", "), count: extra })
      : t("syncplay.watchingWith", { names: displayNames.join(", ") })
    : t("syncplay.watchTogetherRoom");

  return (
    <div
      className="flex items-center gap-3 px-4 py-2 text-sm"
      style={{ background: "var(--overlay-bg-strong)", borderTop: "1px solid var(--overlay-border-subtle)", color: "var(--overlay-text)" }}
    >
      <span
        className="w-2 h-2 rounded-full flex-shrink-0"
        style={{ background: connected ? "var(--color-success)" : "var(--color-danger)" }}
        title={connected ? t("syncplay.connected") : t("syncplay.connecting")}
      />
      <span className="flex-1 truncate">{watchingWith}</span>
      <button
        onClick={handleCopy}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity flex-shrink-0"
        style={{ background: "var(--overlay-btn-bg)" }}
      >
        {copied ? tc("copied") : t("syncplay.shareLink")}
      </button>
      <button
        onClick={onLeave}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity flex-shrink-0"
        style={{ background: "color-mix(in srgb, var(--color-danger) 30%, transparent)" }}
      >
        {t("syncplay.leave")}
      </button>
    </div>
  );
}
