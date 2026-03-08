import { useNotificationServiceGetUnreadCount } from "@evey/api";

interface Props {
  href?: string;
}

export function NotificationBell({ href = "/profile/notifications" }: Props) {
  const { data } = useNotificationServiceGetUnreadCount();
  const count = data?.count ?? 0;

  return (
    <a
      href={href}
      className="relative text-[var(--text-muted)] hover:text-[var(--text)] transition-colors text-lg"
      aria-label="Notifications"
    >
      🔔
      {count > 0 && (
        <span className="absolute -top-1 -right-1 w-4 h-4 bg-red-500 text-white text-xs rounded-full flex items-center justify-center">
          {count > 9 ? "9+" : count}
        </span>
      )}
    </a>
  );
}
