import { createLazyFileRoute } from "@tanstack/react-router";
import { Card } from "@/components/ui/Card";

export const Route = createLazyFileRoute("/_auth/admin/logs")({
  component: LogsPage,
});

const LOG_LEVELS = [
  {
    level: "INFO",
    color: "var(--text)",
    description: "General operational messages",
  },
  {
    level: "WARN",
    color: "#f59e0b",
    description: "Non-critical issues that may need attention",
  },
  {
    level: "ERROR",
    color: "#ef4444",
    description: "Errors that caused a failure",
  },
];

function LogsPage() {
  return (
    <div className="flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold mb-1" style={{ color: "var(--text)" }}>
          Logs
        </h1>
        <p className="text-sm" style={{ color: "var(--text-muted)" }}>
          Server logs are written to disk via lumberjack with automatic rotation.
        </p>
      </div>

      <Card>
        <h2 className="font-semibold mb-3" style={{ color: "var(--text)" }}>
          Log Location
        </h2>
        <p className="text-sm mb-3" style={{ color: "var(--text-muted)" }}>
          Configured via the <code className="px-1 py-0.5 rounded text-xs font-mono"
            style={{ backgroundColor: "var(--surface-raised)", color: "var(--text)" }}>
            EVEY_LOG_DIR
          </code> environment variable. Defaults to <code className="px-1 py-0.5 rounded text-xs font-mono"
            style={{ backgroundColor: "var(--surface-raised)", color: "var(--text)" }}>
            ./logs/
          </code>.
        </p>
        <div
          className="rounded-[var(--radius-md)] p-3 font-mono text-sm"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}
        >
          {"$EVEY_LOG_DIR/evey.log"}
        </div>
      </Card>

      <Card>
        <h2 className="font-semibold mb-3" style={{ color: "var(--text)" }}>
          Log Levels
        </h2>
        <div className="flex flex-col gap-2">
          {LOG_LEVELS.map(({ level, color, description }) => (
            <div
              key={level}
              className="flex items-center gap-3 text-sm py-2"
              style={{ borderBottom: "1px solid var(--border)" }}
            >
              <span
                className="font-mono font-bold w-12"
                style={{ color }}
              >
                {level}
              </span>
              <span style={{ color: "var(--text-muted)" }}>{description}</span>
            </div>
          ))}
        </div>
      </Card>

      <Card>
        <h2 className="font-semibold mb-3" style={{ color: "var(--text)" }}>
          Log Rotation
        </h2>
        <dl className="grid grid-cols-1 sm:grid-cols-3 gap-4 text-sm">
          {[
            ["Max File Size", "EVEY_LOG_MAX_SIZE_MB (default: 100 MB)"],
            ["Retention", "EVEY_LOG_RETENTION_DAYS (default: 30 days)"],
            ["Format", "JSON (production) / colorized console (EVEY_DEV_MODE=true)"],
          ].map(([label, value]) => (
            <div key={label}>
              <dt className="font-medium mb-1" style={{ color: "var(--text-muted)" }}>
                {label}
              </dt>
              <dd style={{ color: "var(--text)" }}>{value}</dd>
            </div>
          ))}
        </dl>
      </Card>

      <Card>
        <h2 className="font-semibold mb-3" style={{ color: "var(--text)" }}>
          Viewing Logs
        </h2>
        <p className="text-sm mb-3" style={{ color: "var(--text-muted)" }}>
          Tail the current log file on your server:
        </p>
        <div
          className="rounded-[var(--radius-md)] p-3 font-mono text-sm"
          style={{ backgroundColor: "var(--surface-raised)", color: "var(--text-muted)" }}
        >
          {"tail -f $EVEY_LOG_DIR/evey.log | jq ."}
        </div>
      </Card>
    </div>
  );
}
