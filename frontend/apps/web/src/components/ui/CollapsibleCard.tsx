import { useState, type ReactNode } from "react";
import { ChevronDown, ChevronRight } from "lucide-react";
import { Card } from "./Card";
import { InfoTooltip } from "./InfoTooltip";
import { ICON } from "@/constants/iconSizes";

interface CollapsibleCardProps {
  title: string;
  subtitle?: string;
  icon?: ReactNode;
  /** Optional long-form explanation. Renders an "i" button next to the title;
      content shows in a popover on click. Use when the subtitle would be too
      long but the operator still benefits from on-demand detail. */
  info?: ReactNode;
  /** When true, the header is decoration only - the body is always shown. */
  alwaysOpen?: boolean;
  /** Initial open state. Ignored when alwaysOpen is true. */
  defaultOpen?: boolean;
  children: ReactNode;
}

// CollapsibleCard wraps any settings section in a uniform expandable card.
// Used in server-config to keep the page short by default - only the
// first card (Paramètres du serveur) stays always-open, the rest collapse.
export function CollapsibleCard({
  title,
  subtitle,
  icon,
  info,
  alwaysOpen = false,
  defaultOpen = false,
  children,
}: CollapsibleCardProps) {
  const [open, setOpen] = useState(defaultOpen);
  const isOpen = alwaysOpen || open;

  const headerProps = alwaysOpen
    ? {}
    : {
        role: "button" as const,
        tabIndex: 0,
        onClick: () => setOpen((v) => !v),
        onKeyDown: (e: React.KeyboardEvent) => {
          if (e.key === "Enter" || e.key === " ") {
            e.preventDefault();
            setOpen((v) => !v);
          }
        },
        "aria-expanded": isOpen,
      };

  return (
    <Card className="p-0">
      <div
        {...headerProps}
        className={`flex items-center gap-3 p-4 ${alwaysOpen ? "" : "cursor-pointer select-none hover:bg-[var(--surface-hover)]"}`}
      >
        {!alwaysOpen && (
          <span aria-hidden="true" className="text-[var(--text-muted)] shrink-0">
            {isOpen ? <ChevronDown size={ICON.lg} /> : <ChevronRight size={ICON.lg} />}
          </span>
        )}
        {icon && (
          <span aria-hidden="true" className="text-[var(--text-muted)] shrink-0">
            {icon}
          </span>
        )}
        <div className="flex-1 min-w-0">
          <h3 className="text-base font-semibold text-[var(--text)] truncate flex items-center gap-1.5">
            {title}
            {info && <InfoTooltip label={title}>{info}</InfoTooltip>}
          </h3>
          {subtitle && (
            <p className="text-xs text-[var(--text-muted)] mt-0.5 truncate">{subtitle}</p>
          )}
        </div>
      </div>
      {isOpen && (
        <div className="border-t border-[var(--border)] p-4">{children}</div>
      )}
    </Card>
  );
}
