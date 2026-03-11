import { createLazyFileRoute } from "@tanstack/react-router";
import { ScrollText } from "lucide-react";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: LogsPage,
});

function LogsPage() {
  return (
    <div className="flex flex-col items-center justify-center py-16 gap-4">
      <ScrollText size={48} style={{ color: "var(--text-muted)" }} />
      <h2 className="text-lg font-semibold" style={{ color: "var(--text)" }}>
        Live Logs
      </h2>
      <p className="text-sm" style={{ color: "var(--text-muted)" }}>
        Coming soon — real-time log viewer with filtering
      </p>
    </div>
  );
}
