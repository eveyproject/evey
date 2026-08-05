import { useMutation } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import { eveyFetch } from "@evey/api";
import { toast } from "@evey/store";
import { Spinner } from "@/components/ui/Spinner";
import { Button } from "@/components/ui/Button";
import { Checkbox } from "@/components/ui/Checkbox";
import { useSetConfigEntry, type ConfigEntry } from "./hooks";

const ARTWORK_AUTO_DOWNLOAD_KEY = "artwork.auto_download";

// Surfaces the runtime toggle for fire-and-forget artwork download on scan,
// plus a one-shot "retry now" button that backfills any rows still missing
// their local copy. Default ON - without local caching the frontend depends
// on external CDN URLs (Amazon for OMDb), some of which Firefox blocks under
// Opaque Response Blocking.
export function ArtworkSettingsCard({ entries }: { entries: ConfigEntry[] | undefined }) {
  const { t } = useTranslation();
  const setEntry = useSetConfigEntry();
  // Default-true semantics: only "false" disables. Missing key or any other
  // value resolves to true to match the server's resolveAutoDownloadDefault.
  const stored = entries?.find((e) => e.key === ARTWORK_AUTO_DOWNLOAD_KEY)?.value;
  const enabled = stored !== "false";

  const retry = useMutation({
    mutationFn: () =>
      // Runs as a pollable background job server-side (a full retry can take
      // minutes); progress is tracked on the Artworks admin page. Here it's a
      // secondary trigger, so we just confirm it started.
      eveyFetch<{ jobId: string; total: number }>({
        url: "/api/v1/admin/artworks/retry-missing",
        method: "POST",
      }),
    onSuccess: () => toast.success(t("admin.serverConfig.artworkRetryStarted")),
    onError: (err: Error) => toast.error(err.message),
  });

  return (
    <div className="flex flex-col gap-3">
        <Checkbox
          label={t("admin.serverConfig.artworkAutoDownload")}
          checked={enabled}
          onChange={(v) =>
            setEntry.mutate({ key: ARTWORK_AUTO_DOWNLOAD_KEY, value: v ? "true" : "false" })
          }
          disabled={setEntry.isPending}
        />
        <div className="flex items-center gap-3 mt-1">
          <Button
            variant="secondary"
            onClick={() => retry.mutate()}
            loading={retry.isPending}
            className="text-xs"
          >
            {t("admin.serverConfig.artworkRetryNow")}
          </Button>
          <span className="text-xs text-[var(--text-muted)]">
            {t("admin.serverConfig.artworkRetryDesc")}
          </span>
        </div>
        {setEntry.isPending && <Spinner size="sm" />}
    </div>
  );
}
