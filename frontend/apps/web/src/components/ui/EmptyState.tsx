import type { ReactNode } from "react";
import { Button } from "./Button";

type IconName = "search" | "library" | "notification" | "history" | "media";

type EmptyStateProps = {
  icon?: IconName;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
};

const icons: Record<IconName, ReactNode> = {
  search: (
    <svg className="w-12 h-12" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <circle cx={11} cy={11} r={7} />
      <path strokeLinecap="round" d="M21 21l-4.35-4.35" />
    </svg>
  ),
  library: (
    <svg className="w-12 h-12" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M3 7v10a2 2 0 002 2h14a2 2 0 002-2V9a2 2 0 00-2-2h-6l-2-2H5a2 2 0 00-2 2z" />
    </svg>
  ),
  notification: (
    <svg className="w-12 h-12" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <path strokeLinecap="round" strokeLinejoin="round" d="M15 17h5l-1.405-1.405A2.032 2.032 0 0118 14.158V11a6.002 6.002 0 00-4-5.659V5a2 2 0 10-4 0v.341C7.67 6.165 6 8.388 6 11v3.159c0 .538-.214 1.055-.595 1.436L4 17h5m6 0v1a3 3 0 11-6 0v-1m6 0H9" />
    </svg>
  ),
  history: (
    <svg className="w-12 h-12" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <circle cx={12} cy={12} r={9} />
      <path strokeLinecap="round" d="M12 7v5l3 3" />
    </svg>
  ),
  media: (
    <svg className="w-12 h-12" fill="none" stroke="currentColor" strokeWidth={1.5} viewBox="0 0 24 24">
      <rect x={2} y={4} width={20} height={16} rx={2} />
      <path strokeLinecap="round" strokeLinejoin="round" d="M10 9l5 3-5 3V9z" />
    </svg>
  ),
};

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 px-4 text-center">
      {icon && (
        <div className="mb-4" style={{ color: "var(--text-muted)" }}>
          {icons[icon]}
        </div>
      )}
      <p className="text-lg font-medium mb-1" style={{ color: "var(--text)" }}>
        {title}
      </p>
      {description && (
        <p className="text-sm max-w-sm" style={{ color: "var(--text-muted)" }}>
          {description}
        </p>
      )}
      {action && (
        <div className="mt-4">
          <Button variant="secondary" onClick={action.onClick}>
            {action.label}
          </Button>
        </div>
      )}
    </div>
  );
}
