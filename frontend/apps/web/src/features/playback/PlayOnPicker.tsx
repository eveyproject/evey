import { useState, useRef, useEffect } from "react";
import { useTranslation } from "react-i18next";
import { Tv, Smartphone, MonitorSpeaker, Loader } from "lucide-react";
import { toast } from "@evey/store";
import { usePlayOnDevices, usePlayOnCommand, type PlayOnDevice } from "@/hooks/usePlayOnDevices";
import { ICON } from "@/constants/iconSizes";

interface PlayOnPickerProps {
  mediaId: string;
  // Read the current playback position at send time (so the target device
  // resumes here). A getter, not a value, because the parent no longer
  // re-renders on every position tick - a captured number would be stale.
  getPositionMs: () => number;
}

function iconFor(type: string) {
  switch (type) {
    case "tv":     return Tv;
    case "mobile": return Smartphone;
    default:       return MonitorSpeaker;
  }
}

// PlayOnPicker - the universal, server-brokered "send this to another
// device" affordance. Unlike Chromecast it works for any browser (no
// Google SDK), unlike DLNA it works for remote users (no LAN dependency)
// - the only requirement is that the target device runs an evey client
// holding a persistent WebSocket to the server.
export function PlayOnPicker({ mediaId, getPositionMs }: PlayOnPickerProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);
  const { data: devices, isLoading, refetch } = usePlayOnDevices();
  const sendCommand = usePlayOnCommand();

  // Close on outside click - same UX as the other player popovers.
  useEffect(() => {
    if (!open) return;
    function onDocClick(e: MouseEvent) {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    }
    document.addEventListener("mousedown", onDocClick);
    return () => document.removeEventListener("mousedown", onDocClick);
  }, [open]);

  // No devices ever registered → hide entirely. An empty list when the
  // popover is open shows "no devices" so the user knows the picker is
  // working but their TV / phone isn't online.
  const hasDevices = (devices?.length ?? 0) > 0;
  if (!isLoading && !hasDevices && !open) {
    // Don't even render the button when there are zero devices - it
    // would be dead-clickable. Refetch happens on its own poll, so the
    // button reappears when a device comes online.
    return null;
  }

  function handleSelect(device: PlayOnDevice) {
    sendCommand.mutate(
      { deviceId: device.id, command: { type: "loadAndPlay", mediaId, positionMs: getPositionMs() } },
      {
        onSuccess: () => {
          toast.success(t("player.playOn.sent", { name: device.name }));
          setOpen(false);
        },
        onError: () => {
          toast.error(t("player.playOn.failed"));
          // Device may have disconnected - refresh so it drops off the list.
          void refetch();
        },
      },
    );
  }

  return (
    <div ref={ref} className="relative">
      <button
        onClick={() => setOpen((v) => !v)}
        aria-label={t("player.playOn.button")}
        aria-expanded={open}
        className="transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded text-[var(--overlay-text)] hover:text-[var(--accent)]"
      >
        <Tv size={ICON.xl} aria-hidden="true" />
      </button>
      {open && (
        <div
          className="absolute bottom-full right-0 mb-2 min-w-[220px] rounded-[var(--radius-md)] shadow-lg z-popover"
          style={{ backgroundColor: "var(--overlay-bg-strong)", border: "1px solid var(--overlay-border-subtle)" }}
        >
          <div className="px-3 py-2 text-xs uppercase tracking-wider text-[var(--text-muted)] border-b border-[var(--overlay-border-subtle)]">
            {t("player.playOn.title")}
          </div>
          {isLoading ? (
            <div className="flex items-center justify-center py-4">
              <Loader size={ICON.md} className="animate-spin text-[var(--overlay-text)]" />
            </div>
          ) : hasDevices ? (
            <ul className="py-1">
              {devices!.map((d) => {
                const Icon = iconFor(d.type);
                return (
                  <li key={d.id}>
                    <button
                      onClick={() => handleSelect(d)}
                      disabled={sendCommand.isPending}
                      className="w-full flex items-center gap-3 px-3 py-2 text-left text-sm text-[var(--overlay-text)] hover:bg-[var(--surface-raised)] disabled:opacity-50"
                    >
                      <Icon size={ICON.lg} aria-hidden="true" />
                      <span className="truncate">{d.name}</span>
                    </button>
                  </li>
                );
              })}
            </ul>
          ) : (
            <div className="px-3 py-3 text-xs text-[var(--text-muted)]">
              {t("player.playOn.empty")}
            </div>
          )}
        </div>
      )}
    </div>
  );
}
