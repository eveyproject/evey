import { useTranslation } from "react-i18next";
import { Tv, Smartphone, MonitorSpeaker, Loader, Cast } from "lucide-react";
import { toast } from "@evey/store";
import { usePlayOnDevices, usePlayOnCommand, type PlayOnDevice } from "@/hooks/usePlayOnDevices";
import { ICON } from "@/constants/iconSizes";

export interface RemotePlaybackMenuProps {
  mediaId: string;
  // Read the current playback position at send time (so the target device
  // resumes here). A getter, not a value, because the parent no longer
  // re-renders on every position tick - a captured number would be stale.
  getPositionMs: () => number;
  open: boolean;
  onToggle: () => void;
  // Whether a Chromecast path exists in THIS browser at all (Cast SDK ready,
  // or the Chrome-Android Remote Playback API is present). Firefox, Safari
  // and iOS never set this - the menu then falls back to Evey devices only,
  // with no empty "Cast" section shown.
  castAvailable?: boolean;
  castConnected?: boolean;
  remoteConnected?: boolean;
  castDeviceName?: string | null;
  // Single tap action, already routed to the right cast transport (Cast SDK
  // start/stop, or the Chrome-Android Remote Playback fling start/stop) -
  // see pickCastTapPath in HLSPlayer/hlsPlayerUtils. This menu never picks
  // the transport itself, it only surfaces the one action as a list entry.
  onCastTrigger?: () => void;
}

function iconFor(type: string) {
  switch (type) {
    case "tv":     return Tv;
    case "mobile": return Smartphone;
    default:       return MonitorSpeaker;
  }
}

// RemotePlaybackMenu - the single "send this playback elsewhere" affordance.
// The player used to expose TWO separate buttons for the same intent: a
// Chromecast icon (Google SDK, Chromium only) and a server-brokered "Play on"
// picker (works in any browser, targets Evey's own TV/mobile clients). Two
// buttons for "send elsewhere" was confusing about which one reaches which
// device, so this merges them into one menu, sections grouped by device
// nature - the model of other media servers. It reimplements neither transport: Chromecast
// still goes through the caller-supplied onCastTrigger, Evey devices still go
// through usePlayOnCommand exactly as the old PlayOnPicker did.
export function RemotePlaybackMenu({
  mediaId,
  getPositionMs,
  open,
  onToggle,
  castAvailable = false,
  castConnected = false,
  remoteConnected = false,
  castDeviceName = null,
  onCastTrigger,
}: RemotePlaybackMenuProps) {
  const { t } = useTranslation();
  const { data: devices, isLoading, refetch } = usePlayOnDevices();
  const sendCommand = usePlayOnCommand();

  const hasDevices = (devices?.length ?? 0) > 0;
  const isCasting = castConnected || remoteConnected;

  // Nothing to offer at all: no Chromecast path in this browser and no Evey
  // device has ever registered. Same guard as the old PlayOnPicker - a
  // trigger that opens on a permanently empty menu is dead-clickable. Poll-driven
  // refetches / SDK readiness flip castAvailable/hasDevices later, at which
  // point the trigger reappears on its own.
  if (!castAvailable && !isLoading && !hasDevices && !open) {
    return null;
  }

  function toggle(e: React.MouseEvent) {
    e.stopPropagation();
    onToggle();
  }

  function handleCastSelect() {
    onCastTrigger?.();
    onToggle();
  }

  function handleDeviceSelect(device: PlayOnDevice) {
    sendCommand.mutate(
      { deviceId: device.id, command: { type: "loadAndPlay", mediaId, positionMs: getPositionMs() } },
      {
        onSuccess: () => {
          toast.success(t("player.playOn.sent", { name: device.name }));
          onToggle();
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
    <div className="relative">
      <button
        onClick={toggle}
        aria-label={t("player.playOn.button")}
        aria-expanded={open}
        aria-haspopup="true"
        title={isCasting && castDeviceName ? castDeviceName : undefined}
        className={`transition-colors-fast p-1 focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1 rounded hover:text-[var(--accent)] ${isCasting ? "text-[var(--accent)]" : "text-[var(--overlay-text)]"}`}
      >
        <Cast size={ICON.lg} aria-hidden="true" />
      </button>
      {open && (
        <div
          className="absolute bottom-full right-0 mb-2 min-w-[220px] rounded-[var(--radius-md)] shadow-lg z-popover"
          style={{ backgroundColor: "var(--overlay-bg-strong)", border: "1px solid var(--overlay-border-subtle)" }}
          onClick={(e) => e.stopPropagation()}
        >
          {castAvailable && (
            <>
              <div className="px-3 py-2 text-xs uppercase tracking-wider text-[var(--text-muted)] border-b border-[var(--overlay-border-subtle)]">
                {t("player.controls.cast")}
              </div>
              <ul className="py-1">
                <li>
                  <button
                    onClick={handleCastSelect}
                    className="w-full flex items-center gap-3 px-3 py-2 text-left text-sm hover:bg-[var(--surface-raised)]"
                    style={{ color: isCasting ? "var(--accent)" : "var(--overlay-text)" }}
                  >
                    <Cast size={ICON.lg} aria-hidden="true" />
                    <span className="truncate">
                      {isCasting && castDeviceName ? castDeviceName : t("player.controls.cast")}
                    </span>
                  </button>
                </li>
              </ul>
            </>
          )}

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
                      onClick={() => handleDeviceSelect(d)}
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
