import { useState } from "react";
import type { SyncPlayMember } from "./useSyncPlay";

interface SyncPlayBarProps {
  roomId: string;
  members: SyncPlayMember[];
  connected: boolean;
  onLeave: () => void;
}

export function SyncPlayBar({ roomId, members, connected, onLeave }: SyncPlayBarProps) {
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
    ? `Watching with: ${displayNames.join(", ")}${extra > 0 ? ` +${extra} more` : ""}`
    : "Watch Together room";

  return (
    <div
      className="flex items-center gap-3 px-4 py-2 text-sm text-white"
      style={{ background: "rgba(0,0,0,0.7)", borderTop: "1px solid rgba(255,255,255,0.1)" }}
    >
      <span
        className="w-2 h-2 rounded-full flex-shrink-0"
        style={{ background: connected ? "#22c55e" : "#ef4444" }}
        title={connected ? "Connected" : "Connecting..."}
      />
      <span className="flex-1 truncate">{watchingWith}</span>
      <button
        onClick={handleCopy}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity flex-shrink-0"
        style={{ background: "rgba(255,255,255,0.15)" }}
      >
        {copied ? "Copied!" : "Share link"}
      </button>
      <button
        onClick={onLeave}
        className="px-3 py-1 rounded text-xs font-medium hover:opacity-80 transition-opacity flex-shrink-0"
        style={{ background: "rgba(239,68,68,0.3)" }}
      >
        Leave
      </button>
    </div>
  );
}
