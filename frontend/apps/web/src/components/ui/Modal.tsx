import { useEffect, useId, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useFocusTrap } from "@/hooks/useFocusTrap";

type ModalSize = "sm" | "md" | "lg" | "xl" | "2xl";

const sizeClasses: Record<ModalSize, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-xl",
  // For panels whose content is images rather than fields - the metadata
  // editor picks artwork, and you cannot choose what you cannot see.
  "2xl": "max-w-2xl",
};

interface ModalProps {
  open: boolean;
  onClose: () => void;
  title?: string;
  size?: ModalSize;
  children: ReactNode;
}

export function Modal({ open, onClose, title, size = "md", children }: ModalProps) {
  const id = useId();
  const titleId = `${id}-modal-title`;
  const bodyId = `${id}-modal-body`;
  const backdropRef = useRef<HTMLDivElement>(null);
  const dialogRef = useRef<HTMLDivElement>(null);

  useFocusTrap(dialogRef, open);

  useEffect(() => {
    if (!open) return;
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    window.addEventListener("keydown", handler);
    return () => window.removeEventListener("keydown", handler);
  }, [open, onClose]);

  if (!open) return null;

  return createPortal(
    <div
      ref={backdropRef}
      className="fixed inset-0 z-overlay flex items-center justify-center bg-black/60"
      onClick={(e) => {
        if (e.target === backdropRef.current) onClose();
      }}
    >
      <div
        ref={dialogRef}
        role="dialog"
        aria-modal="true"
        aria-labelledby={title ? titleId : undefined}
        aria-describedby={bodyId}
        tabIndex={-1}
        className={`bg-[var(--surface)] rounded-[var(--radius-lg)] border border-[var(--border)] p-4 sm:p-6 w-full ${sizeClasses[size]} shadow-xl outline-none max-h-[90dvh] overflow-y-auto pb-[env(safe-area-inset-bottom)]`}
      >
        {title && (
          <h2 id={titleId} className="text-lg font-semibold text-[var(--text)] mb-4">
            {title}
          </h2>
        )}
        <div id={bodyId}>{children}</div>
      </div>
    </div>,
    document.body,
  );
}
