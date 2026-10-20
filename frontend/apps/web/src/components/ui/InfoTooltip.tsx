import { useState, type KeyboardEvent, type ReactNode } from "react";
import { Info, X } from "lucide-react";
import { useTranslation } from "react-i18next";

interface InfoTooltipProps {
  /** Accessible label of the trigger button (e.g. "More info: intro providers"). */
  label: string;
  /** Body of the popover. Pre-formatted JSX so the caller controls layout. */
  children: ReactNode;
}

// activateOnKey makes a span with role="button" behave like a button from the
// keyboard: Enter and Space press it, Space without scrolling the page.
function activateOnKey(e: KeyboardEvent<HTMLElement>, action: () => void) {
  if (e.key === "Enter" || e.key === " ") {
    e.preventDefault();
    e.stopPropagation();
    action();
  }
}

// InfoTooltip renders a small "i" button next to a field title or section
// header. Clicking it pops a small panel anchored below the button with the
// detail content. Backdrop click, Escape and the X button close it. Used to
// keep labels short while still surfacing the plain-language explanation on
// demand.
//
// The trigger and the X are spans with role="button", not <button>s: the
// admin pages sit inside a disabled <fieldset> on the demo (DemoReadOnly),
// which disables every real button beneath it. Reading an explanation is not
// a write, so it must keep working there.
export function InfoTooltip({ label, children }: InfoTooltipProps) {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);

  return (
    <span
      className="relative inline-flex align-middle"
      onKeyDown={(e) => {
        if (open && e.key === "Escape") {
          e.stopPropagation();
          setOpen(false);
        }
      }}
    >
      <span
        role="button"
        tabIndex={0}
        onClick={(e) => {
          // Stop propagation so the tooltip on a clickable header (e.g.
          // CollapsibleCard) doesn't accidentally toggle the parent.
          e.stopPropagation();
          setOpen((v) => !v);
        }}
        onKeyDown={(e) => activateOnKey(e, () => setOpen((v) => !v))}
        aria-label={label}
        aria-expanded={open}
        className="inline-flex cursor-pointer p-0.5 rounded-full text-[var(--text-muted)] hover:text-[var(--accent)] hover:bg-[var(--surface-hover)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-1"
      >
        <Info size={14} aria-hidden="true" />
      </span>
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
            className="absolute left-0 top-full mt-2 z-50 w-80 max-w-[calc(100vw-2rem)] p-3 bg-[var(--surface)] border border-[var(--border)] rounded-[var(--radius-md)] shadow-lg text-sm font-normal normal-case tracking-normal leading-relaxed text-[var(--text)] block whitespace-normal text-left"
          >
            <span className="flex items-start justify-between gap-2">
              <span className="flex-1 block">{children}</span>
              <span
                role="button"
                tabIndex={0}
                onClick={(e) => {
                  e.stopPropagation();
                  setOpen(false);
                }}
                onKeyDown={(e) => activateOnKey(e, () => setOpen(false))}
                aria-label={t("common.aria.close")}
                className="inline-flex cursor-pointer p-0.5 -m-0.5 rounded text-[var(--text-muted)] hover:text-[var(--text)] focus:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)]"
              >
                <X size={14} aria-hidden="true" />
              </span>
            </span>
          </span>
        </>
      )}
    </span>
  );
}
