import { useMemo, useState } from "react";
import { createLazyFileRoute } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useQueryClient } from "@tanstack/react-query";
import { Inbox } from "lucide-react";
import {
  useAdminMediaRequestServiceListRequests,
  useAdminMediaRequestServiceAnswerRequest,
  useAdminMediaRequestServiceGetAccessSettings,
  useAdminMediaRequestServiceSetAccessDefault,
  useAdminMediaRequestServiceSetUserAccess,
  getAdminMediaRequestServiceListRequestsQueryKey,
  getAdminMediaRequestServiceGetAccessSettingsQueryKey,
  useUserServiceListUsers,
  V1AccessMode,
  type V1MediaRequestProto,
} from "@evey/api";
import { toast } from "@evey/store";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Badge } from "@/components/ui/Badge";
import { Textarea } from "@/components/ui/Textarea";
import { Select } from "@/components/ui/Select";
import { Switch } from "@/components/ui/Switch";
import { Spinner } from "@/components/ui/Spinner";
import { EmptyState } from "@/components/ui/EmptyState";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { RequestPoster } from "@/features/requests/RequestPoster";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/media-requests")({
  component: AdminMediaRequestsPage,
});

type AdminTab = "queue" | "access";
type StatusFilter = "all" | "pending" | "approved" | "rejected" | "later";

const statusVariant: Record<string, "default" | "success" | "warning" | "error"> = {
  pending: "default",
  approved: "success",
  rejected: "error",
  later: "warning",
};

function capitalize(s?: string) {
  if (!s) return "Pending";
  return s.charAt(0).toUpperCase() + s.slice(1);
}

function RequestRow({ request }: { request: V1MediaRequestProto }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [note, setNote] = useState("");

  const answer = useAdminMediaRequestServiceAnswerRequest({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: getAdminMediaRequestServiceListRequestsQueryKey() });
      },
      onError: (_err, vars) => {
        const key = vars.data.status === "approved" ? "approveFailed" : vars.data.status === "rejected" ? "rejectFailed" : "laterFailed";
        toast.error(t(`admin.mediaRequests.${key}`));
      },
    },
  });

  function handleAnswer(status: "approved" | "rejected" | "later") {
    if (!request.id) return;
    answer.mutate({ id: request.id, data: { status, note: note.trim() || undefined } });
  }

  const isPending = request.status === "pending";

  return (
    <Card className="flex gap-3 items-start">
      <div className="w-12 h-18 rounded-[var(--radius-sm)] overflow-hidden flex-shrink-0">
        <RequestPoster posterUrl={request.posterUrl} title={request.title} mediaType={request.mediaType} />
      </div>
      <div className="flex-1 min-w-0 flex flex-col gap-2">
        <div className="flex items-center gap-2 flex-wrap">
          <span className="font-medium text-[var(--text)]">{request.title}</span>
          {request.year != null && request.year > 0 && (
            <span className="text-xs text-[var(--text-muted)]">({request.year})</span>
          )}
          <Badge variant={statusVariant[request.status ?? "pending"] ?? "default"}>
            {t(`requests.status${capitalize(request.status)}`)}
          </Badge>
        </div>
        {request.requesterName && (
          <p className="text-xs text-[var(--text-muted)]">
            {t("admin.mediaRequests.requestedBy", { name: request.requesterName })}
          </p>
        )}
        {isPending ? (
          <div className="flex flex-col gap-2 pt-1">
            <Textarea
              value={note}
              onChange={(e) => setNote(e.target.value)}
              placeholder={t("admin.mediaRequests.notePlaceholder")}
              rows={2}
            />
            <div className="flex gap-2">
              <Button variant="primary" loading={answer.isPending} onClick={() => handleAnswer("approved")} {...lock}>
                {t("admin.mediaRequests.approve")}
              </Button>
              <Button variant="danger" loading={answer.isPending} onClick={() => handleAnswer("rejected")} {...lock}>
                {t("admin.mediaRequests.reject")}
              </Button>
              <Button variant="secondary" loading={answer.isPending} onClick={() => handleAnswer("later")} {...lock}>
                {t("admin.mediaRequests.later")}
              </Button>
            </div>
          </div>
        ) : (
          request.adminNote && (
            <p className="text-xs text-[var(--text-muted)]">{t("admin.mediaRequests.answeredNote", { note: request.adminNote })}</p>
          )
        )}
      </div>
    </Card>
  );
}

function QueueTab() {
  const { t } = useTranslation();
  const [filter, setFilter] = useState<StatusFilter>("pending");

  const { data, isLoading, isError, error, refetch } = useAdminMediaRequestServiceListRequests(
    filter === "all" ? {} : { statuses: [filter] },
  );
  const items = data?.items ?? [];

  const filters: StatusFilter[] = ["all", "pending", "approved", "rejected", "later"];

  return (
    <div className="flex flex-col gap-4">
      <div className="flex gap-2 flex-wrap">
        {filters.map((f) => (
          <button
            key={f}
            type="button"
            onClick={() => setFilter(f)}
            className="px-3 py-1.5 rounded-[var(--radius-md)] text-sm font-medium transition-colors-fast"
            style={{
              backgroundColor: filter === f ? "var(--accent)" : "var(--surface-raised)",
              color: filter === f ? "var(--filter-active-text)" : "var(--text-muted)",
            }}
            aria-pressed={filter === f}
          >
            {t(`admin.mediaRequests.filter${capitalize(f)}`)}
          </button>
        ))}
      </div>

      {isLoading ? (
        <div className="flex justify-center py-10">
          <Spinner />
        </div>
      ) : isError ? (
        <FetchErrorState error={error} onRetry={() => void refetch()} />
      ) : items.length === 0 ? (
        <EmptyState icon={<Inbox size={48} strokeWidth={1.5} />} title={t("admin.mediaRequests.empty")} />
      ) : (
        <div className="flex flex-col gap-2">
          {items.map((r) => (
            <RequestRow key={r.id} request={r} />
          ))}
        </div>
      )}
    </div>
  );
}

function AccessTab() {
  const { t } = useTranslation();
  const { demo, lock } = useDemoLock();
  const queryClient = useQueryClient();
  const { data: settings, isLoading } = useAdminMediaRequestServiceGetAccessSettings();
  const { data: usersData } = useUserServiceListUsers();
  const [addUserId, setAddUserId] = useState("");
  const [addMode, setAddMode] = useState<string>(V1AccessMode.ACCESS_MODE_ENABLED);

  const setDefault = useAdminMediaRequestServiceSetAccessDefault({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.mediaRequests.accessDefaultUpdated"));
        void queryClient.invalidateQueries({ queryKey: getAdminMediaRequestServiceGetAccessSettingsQueryKey() });
      },
      onError: () => toast.error(t("admin.mediaRequests.accessDefaultFailed")),
    },
  });

  const setUserAccess = useAdminMediaRequestServiceSetUserAccess({
    mutation: {
      onSuccess: (_data, vars) => {
        const name = usersData?.users?.find((u) => u.id === vars.userId)?.username ?? vars.userId;
        toast.success(t("admin.mediaRequests.accessOverrideUpdated", { name }));
        void queryClient.invalidateQueries({ queryKey: getAdminMediaRequestServiceGetAccessSettingsQueryKey() });
        setAddUserId("");
      },
      onError: () => toast.error(t("admin.mediaRequests.accessOverrideFailed")),
    },
  });

  const overrides = useMemo(() => settings?.overrides ?? [], [settings?.overrides]);
  const overriddenIds = useMemo(() => new Set(overrides.map((o) => o.userId)), [overrides]);
  const availableUsers = (usersData?.users ?? []).filter((u) => u.id && !overriddenIds.has(u.id));

  const modeOptions = [
    { value: V1AccessMode.ACCESS_MODE_ENABLED, label: t("admin.mediaRequests.accessModeEnabled") },
    { value: V1AccessMode.ACCESS_MODE_DISABLED, label: t("admin.mediaRequests.accessModeDisabled") },
  ];

  if (isLoading) {
    return (
      <div className="flex justify-center py-10">
        <Spinner />
      </div>
    );
  }

  return (
    <div className="flex flex-col gap-4">
      <Card className="flex items-center justify-between gap-4">
        <div>
          <p className="text-sm font-medium text-[var(--text)]">{t("admin.mediaRequests.accessDefaultLabel")}</p>
          <p className="text-xs text-[var(--text-muted)] mt-0.5">{t("admin.mediaRequests.accessDefaultDesc")}</p>
        </div>
        <Switch
          checked={settings?.defaultEnabled ?? false}
          onChange={() => setDefault.mutate({ data: { enabled: !(settings?.defaultEnabled ?? false) } })}
          label={t("admin.mediaRequests.accessDefaultLabel")}
          disabled={setDefault.isPending || demo}
        />
      </Card>

      <Card className="flex flex-col gap-3">
        <p className="text-sm font-medium text-[var(--text)]">{t("admin.mediaRequests.accessOverridesTitle")}</p>

        {overrides.length === 0 ? (
          <p className="text-xs text-[var(--text-muted)]">{t("admin.mediaRequests.noOverrides")}</p>
        ) : (
          <div className="flex flex-col gap-2">
            {overrides.map((o) => (
              <div key={o.userId} className="flex items-center gap-3">
                <span className="flex-1 text-sm text-[var(--text)]">{o.username}</span>
                <Select
                  value={o.enabled ? V1AccessMode.ACCESS_MODE_ENABLED : V1AccessMode.ACCESS_MODE_DISABLED}
                  onChange={(mode) => {
                    if (!o.userId) return;
                    setUserAccess.mutate({ userId: o.userId, data: { mode: mode as V1AccessMode } });
                  }}
                  options={[
                    ...modeOptions,
                    { value: V1AccessMode.ACCESS_MODE_DEFAULT, label: t("admin.mediaRequests.accessModeDefault") },
                  ]}
                  className="w-40"
                />
              </div>
            ))}
          </div>
        )}

        {availableUsers.length > 0 && (
          <div className="flex items-end gap-2 pt-2 border-t border-[var(--border)] mt-1">
            <div className="flex-1">
              <Select
                label={t("admin.mediaRequests.addOverride")}
                value={addUserId}
                onChange={setAddUserId}
                placeholder={t("admin.mediaRequests.selectUser")}
                options={availableUsers.map((u) => ({ value: u.id ?? "", label: u.username ?? u.id ?? "" }))}
              />
            </div>
            <Select value={addMode} onChange={setAddMode} options={modeOptions} className="w-32" />
            <Button
              type="button"
              disabled={!addUserId || setUserAccess.isPending}
              onClick={() => setUserAccess.mutate({ userId: addUserId, data: { mode: addMode as V1AccessMode } })}
              {...lock}
            >
              {t("admin.mediaRequests.addOverride")}
            </Button>
          </div>
        )}
      </Card>
    </div>
  );
}

function AdminMediaRequestsPage() {
  const { t } = useTranslation();
  const [tab, setTab] = useState<AdminTab>("queue");

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h2 className="text-xl font-semibold text-[var(--text)]">{t("admin.mediaRequests.title")}</h2>
        <p className="text-sm text-[var(--text-muted)] mt-1">{t("admin.mediaRequests.subtitle")}</p>
      </div>

      <div className="flex gap-1 border-b border-[var(--border)]">
        {(["queue", "access"] as AdminTab[]).map((tb) => (
          <button
            key={tb}
            onClick={() => setTab(tb)}
            className={`px-4 py-2 text-sm transition-colors-fast border-b-2 -mb-px ${
              tab === tb
                ? "border-[var(--accent)] text-[var(--accent)]"
                : "border-transparent text-[var(--text-muted)] hover:text-[var(--text)]"
            }`}
          >
            {t(tb === "queue" ? "admin.mediaRequests.tabQueue" : "admin.mediaRequests.tabAccess")}
          </button>
        ))}
      </div>

      {tab === "queue" ? <QueueTab /> : <AccessTab />}
    </div>
  );
}
