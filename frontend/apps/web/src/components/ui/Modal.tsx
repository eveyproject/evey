import { createContext, useContext, useEffect, useId, useRef, type ReactNode } from "react";
import { createPortal } from "react-dom";
import { useFocusTrap } from "@/hooks/useFocusTrap";

type ModalSize = "sm" | "md" | "lg" | "xl" | "2xl" | "3xl" | "4xl";

const sizeClasses: Record<ModalSize, string> = {
  sm: "max-w-sm",
  md: "max-w-md",
  lg: "max-w-lg",
  xl: "max-w-xl",
  // For panels whose content is images rather than fields - the metadata
  // editor picks artwork, and you cannot choose what you cannot see.
  "2xl": "max-w-2xl",
  "3xl": "max-w-3xl",
  "4xl": "max-w-4xl",
};

// Open modals, module-level so modals rendered by unrelated components still
// agree on which one is on top. "On top" is the deepest one in the React tree
// (a modal rendered inside another's children), then the most recently opened:
// registration order alone would not do, since React runs a child's effects
// before its parent's and would crown the parent.
const openModals: { token: symbol; depth: number; seq: number }[] = [];
let modalSeq = 0;
const ModalDepthContext = createContext(0);

function isTopmost(token: symbol): boolean {
  let top: (typeof openModals)[number] | undefined;
  for (const m of openModals) {
    if (!top || m.depth > top.depth || (m.depth === top.depth && m.seq > top.seq)) top = m;
  }
  return top?.token === token;
}

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
  const depth = useContext(ModalDepthContext);

  useFocusTrap(dialogRef, open);

  useEffect(() => {
    if (!open) return;
    // Only the topmost open modal answers Escape: a modal opened from another
    // (Identify inside the edit form) used to close both at once, taking the
    // unsaved edit with it.
    const token = Symbol("modal");
    openModals.push({ token, depth, seq: ++modalSeq });
    const handler = (e: KeyboardEvent) => {
      if (e.key === "Escape" && isTopmost(token)) onClose();
    };
    window.addEventListener("keydown", handler);
    return () => {
      window.removeEventListener("keydown", handler);
      const at = openModals.findIndex((m) => m.token === token);
      if (at >= 0) openModals.splice(at, 1);
    };
  }, [open, onClose, depth]);

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
        <div id={bodyId}>
          <ModalDepthContext.Provider value={depth + 1}>{children}</ModalDepthContext.Provider>
        </div>
      </div>
    </div>,
    document.body,
  );
}
