import { useEffect } from "react";
import { useToastStore } from "@evey/store";
import { Icon } from "./icons";
import type { IoniconName } from "./ionicons";

const TOAST_ICONS: Record<string, IoniconName> = {
  success: "checkmark-circle",
  error: "alert-circle",
  info: "information-circle",
  warning: "warning",
};

// One toast, removed after its duration (3 s unless it says otherwise): the
// store keeps them until a display clears them, as the Android television does.
function ToastItem({ id, type, message, duration }: { id: string; type: string; message: string; duration?: number }) {
  const remove = useToastStore((s) => s.removeToast);
  useEffect(() => {
    const timer = setTimeout(() => remove(id), duration ?? 3000);
    return () => clearTimeout(timer);
  }, [id, duration, remove]);
  return (
    <div className={`toast ${type}`} role="status">
      <Icon name={TOAST_ICONS[type] ?? "information-circle"} size={24} />
      <span className="toast-message">{message}</span>
    </div>
  );
}

// The last three, top centre, the type in a coloured edge and an icon.
export function Toasts() {
  const toasts = useToastStore((s) => s.toasts).slice(-3);
  if (toasts.length === 0) return null;
  return (
    <div className="toast-stack">
      {toasts.map((toast) => (
        <ToastItem key={toast.id} id={toast.id} type={toast.type ?? "info"} message={toast.message} duration={toast.duration} />
      ))}
    </div>
  );
}
