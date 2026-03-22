import type { ReactNode } from "react";
import { Search, Folder, Bell, Clock, Film } from "lucide-react";
import { Button } from "./Button";

type IconName = "search" | "library" | "notification" | "history" | "media";

type EmptyStateProps = {
  icon?: IconName;
  title: string;
  description?: string;
  action?: { label: string; onClick: () => void };
};

const icons: Record<IconName, ReactNode> = {
  search: <Search size={48} strokeWidth={1.5} />,
  library: <Folder size={48} strokeWidth={1.5} />,
  notification: <Bell size={48} strokeWidth={1.5} />,
  history: <Clock size={48} strokeWidth={1.5} />,
  media: <Film size={48} strokeWidth={1.5} />,
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
