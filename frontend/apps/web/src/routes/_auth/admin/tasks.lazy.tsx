import { createLazyFileRoute } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import {
  useAdminServiceListScheduledTasks,
  useAdminServiceRunScheduledTask,
  useAdminServiceUpdateScheduledTask,
} from "@evey/api";
import { toast } from "@evey/store";
import type { V1ScheduledTask } from "@evey/api/src/generated/admin.schemas";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Card } from "@/components/ui/Card";
import { EmptyState } from "@/components/ui/EmptyState";

export const Route = createLazyFileRoute("/_auth/admin/tasks")({
  component: TasksPage,
});

const TASKS_QUERY_KEY = ["/api/v1/admin/tasks"];

function stateVariant(state?: string): "success" | "error" | "warning" | "info" | "default" {
  switch (state) {
    case "TASK_STATE_RUNNING": return "info";
    case "TASK_STATE_COMPLETED": return "success";
    case "TASK_STATE_FAILED": return "error";
    case "TASK_STATE_IDLE": return "default";
    default: return "default";
  }
}

function stateLabel(state?: string): string {
  switch (state) {
    case "TASK_STATE_RUNNING": return "Running";
    case "TASK_STATE_COMPLETED": return "Completed";
    case "TASK_STATE_FAILED": return "Failed";
    case "TASK_STATE_IDLE": return "Idle";
    default: return "Unknown";
  }
}

function formatDate(d?: string): string {
  if (!d) return "\u2014";
  return new Date(d).toLocaleString();
}

function TaskRow({ task }: { task: V1ScheduledTask }) {
  const queryClient = useQueryClient();
  const invalidate = () => queryClient.invalidateQueries({ queryKey: TASKS_QUERY_KEY });

  const runTask = useAdminServiceRunScheduledTask({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Task started"); },
      onError: (err: Error) => toast.error(err.message || "Failed to run task"),
    },
  });
  const updateTask = useAdminServiceUpdateScheduledTask({
    mutation: {
      onSuccess: () => { invalidate(); toast.success("Task updated"); },
      onError: (err: Error) => toast.error(err.message || "Failed to update task"),
    },
  });

  const isRunning = task.state === "TASK_STATE_RUNNING";

  return (
    <Card className="flex flex-col gap-3">
      <div className="flex items-start justify-between gap-4">
        <div className="flex-1 min-w-0">
          <div className="flex items-center gap-2 mb-1">
            <span className="font-medium text-sm" style={{ color: "var(--text)" }}>
              {task.name}
            </span>
            <Badge variant={stateVariant(task.state)}>{stateLabel(task.state)}</Badge>
            {!task.enabled && <Badge variant="error">Disabled</Badge>}
          </div>
          {task.description && (
            <p className="text-xs" style={{ color: "var(--text-muted)" }}>
              {task.description}
            </p>
          )}
        </div>
        <div className="flex gap-2 flex-shrink-0">
          <Button
            variant="secondary"
            loading={updateTask.isPending}
            onClick={() =>
              updateTask.mutate({
                id: task.id!,
                data: { enabled: !task.enabled },
              })
            }
          >
            {task.enabled ? "Disable" : "Enable"}
          </Button>
          <Button
            variant="primary"
            loading={runTask.isPending || isRunning}
            disabled={isRunning}
            onClick={() => runTask.mutate({ id: task.id! })}
          >
            Run Now
          </Button>
        </div>
      </div>

      <dl className="grid grid-cols-2 sm:grid-cols-4 gap-x-6 gap-y-1 text-xs">
        <div>
          <dt style={{ color: "var(--text-muted)" }}>Schedule</dt>
          <dd style={{ color: "var(--text)" }} className="font-mono">
            {task.cronExpression ?? "\u2014"}
          </dd>
        </div>
        <div>
          <dt style={{ color: "var(--text-muted)" }}>Last run</dt>
          <dd style={{ color: "var(--text)" }}>{formatDate(task.lastRunAt)}</dd>
        </div>
        <div>
          <dt style={{ color: "var(--text-muted)" }}>Next run</dt>
          <dd style={{ color: "var(--text)" }}>{formatDate(task.nextRunAt)}</dd>
        </div>
        {task.lastError && (
          <div className="col-span-2">
            <dt style={{ color: "var(--text-muted)" }}>Last error</dt>
            <dd className="text-red-400 truncate">{task.lastError}</dd>
          </div>
        )}
        {task.state === "TASK_STATE_RUNNING" && task.progressPercent !== undefined && (
          <div className="col-span-4">
            <dt style={{ color: "var(--text-muted)" }} className="mb-1">
              Progress
            </dt>
            <div className="h-1.5 rounded-full" style={{ backgroundColor: "var(--border)" }}>
              <div
                className="h-1.5 rounded-full transition-all"
                style={{ width: `${task.progressPercent}%`, backgroundColor: "var(--accent)" }}
              />
            </div>
          </div>
        )}
      </dl>
    </Card>
  );
}

function TasksPage() {
  const { data, isLoading } = useAdminServiceListScheduledTasks();
  const tasks = data?.tasks ?? [];

  return (
    <div className="flex flex-col gap-6">
      <h1 className="text-2xl font-bold" style={{ color: "var(--text)" }}>
        Scheduled Tasks
      </h1>

      {isLoading ? (
        <div className="flex justify-center py-16">
          <Spinner size="lg" />
        </div>
      ) : tasks.length === 0 ? (
        <div
          className="rounded-[var(--radius-lg)]"
          style={{ backgroundColor: "var(--surface)", border: "1px solid var(--border)" }}
        >
          <EmptyState icon="history" title="No scheduled tasks found" />
        </div>
      ) : (
        <div className="flex flex-col gap-4">
          {tasks.map((task) => (
            <TaskRow key={task.id} task={task} />
          ))}
        </div>
      )}
    </div>
  );
}
