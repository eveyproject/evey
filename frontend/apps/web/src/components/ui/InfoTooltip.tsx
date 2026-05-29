import { useState, type ReactNode } from "react";
import { HelpCircle, X } from "lucide-react";

interface InfoTooltipProps {
  /** Accessible label of the trigger button (e.g. "More info: intro providers"). */
  label: string;
  /** Body of the popover. Pre-formatted JSX so the caller controls layout. */
  children: ReactNode;
}

// InfoTooltip renders a small "?" button next to a field title or section
// header. Clicking it pops a small panel anchored below the button with the
// detail content. Backdrop click and the X button close it. Used to keep
// section subtitles short while still surfacing the long-form explanation
// on demand.
export function InfoTooltip({ label, children }: InfoTooltipProps) {
  const [open, setOpen] = useState(false);

  return (
    <span className="relative inline-flex align-middle">
      <button
        type="button"
        onClick={(e) => {
          // Stop propagation so the tooltip on a clickable header (e.g.
          // CollapsibleCard) doesn't accidentally toggle the parent.
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        aria-label={label}
        aria-expanded={open}
        className="p-0.5 rounded-full text-[var(--text-muted)] hover:text-[var(--text)] hover:bg-[var(--surface-hover)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)] focus:ring-offset-1"
      >
        <HelpCircle size={14} aria-hidden="true" />
      </button>
      {open && (
        <>
          {/* Click-outside catcher. fixed full-screen so it absorbs clicks
              regardless of where the popover ended up. */}
          <span
            aria-hidden="true"
            className="fixed inset-0 z-40"
            onClick={(e) => {
              e.stopPropagation();
              setOpen(false);
            }}
          />
          <span
            role="dialog"
            aria-label={label}
            onClick={(e) => e.stopPropagation()}
            className="absolute left-0 top-full mt-2 z-50 w-80 max-w-[80vw] p-3 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-md)] shadow-lg text-sm text-[var(--text)] block whitespace-normal"
          >
            <span className="flex items-start justify-between gap-2">
              <span className="flex-1 block">{children}</span>
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  setOpen(false);
                }}
                aria-label="Close"
                className="p-0.5 -m-0.5 rounded text-[var(--text-muted)] hover:text-[var(--text)] focus:outline-none focus:ring-2 focus:ring-[var(--accent)]"
              >
                <X size={14} aria-hidden="true" />
              </button>
            </span>
          </span>
        </>
      )}
    </span>
  );
}
