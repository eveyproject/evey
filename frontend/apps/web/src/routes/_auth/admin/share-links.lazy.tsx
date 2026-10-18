import { useState } from "react";
import { createLazyFileRoute, Link } from "@tanstack/react-router";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useShareServiceListShareLinks,
  useShareServiceRevokeShareLink,
  getShareServiceListShareLinksQueryKey,
  useLibraryServiceGetMediaItem,
  apiErrorMessage,
  type shareServiceListShareLinks,
} from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Modal } from "@/components/ui/Modal";
import { FetchErrorState } from "@/components/ui/FetchErrorState";
import { useDemoLock } from "@/hooks/useDemoMode";

export const Route = createLazyFileRoute("/_auth/admin/share-links")({
  component: ShareLinksPage,
});

type ShareLink = NonNullable<Awaited<ReturnType<typeof shareServiceListShareLinks>>["items"]>[number];

// A share link hands a title to someone without an account: it is created from
// the title page, and this is where an admin sees every live one and takes one
// back. Without it a link shared by mistake stayed valid until it expired.
function ShareLinkRow({ link }: { link: ShareLink }) {
  const { t, i18n } = useTranslation();
  const { lock } = useDemoLock();
  const queryClient = useQueryClient();
  const [confirming, setConfirming] = useState(false);
  const { data: media } = useLibraryServiceGetMediaItem(link.mediaItemId ?? "", {
    query: { enabled: !!link.mediaItemId, retry: false },
  });
  const title = media?.title || link.mediaItemId || "-";

  const revoke = useShareServiceRevokeShareLink({
    mutation: {
      onSuccess: () => {
        void queryClient.invalidateQueries({ queryKey: getShareServiceListShareLinksQueryKey() });
        toast.success(t("admin.shareLinks.revoked"));
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.shareLinks.failedRevoke"))),
    },
  });

  const expiresAt = link.expiresAt ? new Date(link.expiresAt) : null;
  const expired = expiresAt !== null && expiresAt.getTime() < Date.now();
  const views = link.maxViews != null
    ? `${link.viewCount ?? 0} / ${link.maxViews}`
    : `${link.viewCount ?? 0} / ${t("admin.shareLinks.unlimited")}`;

  return (
    <tr className="border-b border-[var(--border)] last:border-0">
      <td className="px-4 py-3" data-label={t("admin.shareLinks.tableHeaders.media")}>
        {link.mediaItemId ? (
          <Link to="/media/$mediaId" params={{ mediaId: link.mediaItemId }} className="text-[var(--text)] hover:underline">
            {title}
          </Link>
        ) : title}
      </td>
      <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]" data-label={t("admin.shareLinks.tableHeaders.expires")}>
        {expiresAt ? expiresAt.toLocaleString(i18n.language) : "-"}
        {expired && <span className="ml-2 text-[var(--warning)]">{t("admin.shareLinks.expired")}</span>}
      </td>
      <td className="px-4 py-3 whitespace-nowrap text-[var(--text-muted)]" data-label={t("admin.shareLinks.tableHeaders.views")}>
        {views}
      </td>
      <td className="px-4 py-3" data-label={t("admin.shareLinks.tableHeaders.actions")}>
        <Button variant="danger" loading={revoke.isPending} onClick={() => setConfirming(true)} {...lock}>
          {t("admin.shareLinks.revoke")}
        </Button>
        <Modal open={confirming} onClose={() => setConfirming(false)} title={t("admin.shareLinks.revokeTitle")}>
          <p className="text-sm mb-4 text-[var(--text-muted)]">{t("admin.shareLinks.revokeConfirm", { title })}</p>
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setConfirming(false)}>{t("common.cancel")}</Button>
            <Button
              variant="danger"
              loading={revoke.isPending}
              onClick={() => {
                if (link.id) revoke.mutate({ id: link.id });
                setConfirming(false);
              }}
            >
              {t("admin.shareLinks.revoke")}
            </Button>
          </div>
        </Modal>
      </td>
    </tr>
  );
}

function ShareLinksPage() {
  const { t } = useTranslation();
  const { data, isLoading, isError, error, refetch } = useShareServiceListShareLinks();
  const links = data?.items ?? [];
  const headers = [
    t("admin.shareLinks.tableHeaders.media"),
    t("admin.shareLinks.tableHeaders.expires"),
    t("admin.shareLinks.tableHeaders.views"),
    t("admin.shareLinks.tableHeaders.actions"),
  ];

  return (
    <div className="px-6 py-8 max-w-5xl flex flex-col gap-6">
      <div>
        <h1 className="text-2xl font-bold text-[var(--text)]">{t("admin.shareLinks.title")}</h1>
        <p className="text-sm text-[var(--text-muted)] mt-1">{t("admin.shareLinks.description")}</p>
      </div>
      {isLoading ? (
        <div className="flex justify-center py-16"><Spinner size="lg" /></div>
      ) : isError ? (
        <FetchErrorState error={error} onRetry={() => void refetch()} />
      ) : (
        <div className="overflow-x-auto rounded-[var(--radius-lg)] border border-[var(--border)]">
          <table className="w-full text-sm table-cards">
            <thead>
              <tr className="bg-[var(--surface)] border-b border-[var(--border)]">
                {headers.map((h) => (
                  <th key={h} className="px-4 py-2 text-left font-medium text-[var(--text-muted)]">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {links.length === 0 ? (
                <tr>
                  <td colSpan={headers.length} className="px-4 py-8 text-center text-[var(--text-muted)]">
                    {t("admin.shareLinks.noLinks")}
                  </td>
                </tr>
              ) : (
                links.map((link) => <ShareLinkRow key={link.id} link={link} />)
              )}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}
