import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { TriangleAlert } from "lucide-react";
import { eveyFetch, apiErrorMessage } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { useDemoLock } from "@/hooks/useDemoMode";
import { ICON } from "@/constants/iconSizes";

interface PruneSuspension {
  libraryId: string;
  root: string;
  missing: number;
  total: number;
}

const QUERY_KEY = "/api/v1/admin/libraries/prune-suspended";

// A root that lost many files at once keeps its items: the server holds the
// deletion (a pool missing a disk looks exactly like a mass delete) until an
// admin says the files are really gone.
export function PruneSuspendedBanner({ libraries }: { libraries: { id?: string; name?: string }[] }) {
  const { t } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [pending, setPending] = useState<PruneSuspension | null>(null);

  const { data = [] } = useQuery<PruneSuspension[]>({
    queryKey: [QUERY_KEY],
    queryFn: async () => {
      const json = await eveyFetch<{ suspensions: PruneSuspension[] }>({ url: QUERY_KEY, method: "GET" });
      return json.suspensions ?? [];
    },
    refetchInterval: 30_000,
  });

  const confirm = useMutation({
    mutationFn: (libraryId: string) =>
      eveyFetch({ url: `/api/v1/admin/libraries/${libraryId}/prune/confirm`, method: "POST", data: {} }),
    onSuccess: () => {
      toast.success(t("admin.libraries.pruneConfirmStarted"));
      setPending(null);
      queryClient.invalidateQueries({ queryKey: [QUERY_KEY] });
      queryClient.invalidateQueries({ queryKey: ["/api/v1/admin/library-stats"] });
    },
    onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("common.error"))),
  });

  if (data.length === 0) return null;

  return (
    <div className="flex flex-col gap-3" data-testid="prune-suspended">
      {data.map((s) => {
        const name = libraries.find((l) => l.id === s.libraryId)?.name;
        return (
          <div
            key={`${s.libraryId}:${s.root}`}
            className="flex flex-col gap-2 rounded-[var(--radius-lg)] border border-[var(--warning)] bg-[var(--surface)] p-4"
            role="alert"
            data-testid={`prune-suspended-${s.libraryId}-${s.root}`}
          >
            <p className="flex flex-wrap items-center gap-2 text-sm font-semibold text-[var(--text)]">
              <TriangleAlert size={ICON.md} className="text-[var(--warning)]" aria-hidden="true" />
              {name && <span>{name}</span>}
              <span>{t("admin.libraries.pruneSuspended", { missing: s.missing, root: s.root })}</span>
            </p>
            <p className="text-xs text-[var(--text-muted)]">{t("admin.libraries.pruneSuspendedHint")}</p>
            <div>
              <Button variant="danger" onClick={() => setPending(s)} {...lock}>
                {t("admin.libraries.confirmPrune")}
              </Button>
            </div>
          </div>
        );
      })}
      <Modal open={pending !== null} onClose={() => setPending(null)} title={t("admin.libraries.confirmPruneTitle")}>
        <p className="mb-4 text-sm text-[var(--text-muted)]">
          {pending && t("admin.libraries.confirmPruneBody", { missing: pending.missing, root: pending.root })}
        </p>
        <div className="flex justify-end gap-2">
          <Button variant="ghost" onClick={() => setPending(null)}>{t("common.cancel")}</Button>
          <Button
            variant="danger"
            loading={confirm.isPending}
            onClick={() => pending && confirm.mutate(pending.libraryId)}
          >
            {t("admin.libraries.confirmPrune")}
          </Button>
        </div>
      </Modal>
    </div>
  );
}
