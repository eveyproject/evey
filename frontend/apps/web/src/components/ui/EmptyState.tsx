import type { ReactNode } from "react";
import { Button } from "./Button";

type EmptyStateProps = {
  icon?: ReactNode;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
};

export function EmptyState({ icon, title, description, action }: EmptyStateProps) {
  return (
    <div className="flex flex-col items-center justify-center py-16 px-4 text-center">
      {icon && (
        <div className="mb-4" style={{ color: "var(--text-muted)" }}>
          {icon}
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
