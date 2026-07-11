import { Share2 } from "lucide-react";
import { useTranslation } from "react-i18next";
import { useShareServiceCreateShareLink } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";

// Admin-only: mints a PUBLIC (unauthenticated) download link for the media
// file and copies it to the clipboard. The backend gates creation to admins;
// this button is also only rendered for admins.
export function ShareLinkButton({ mediaId }: { mediaId: string }) {
  const { t } = useTranslation();
  const createLink = useShareServiceCreateShareLink({
    mutation: {
      onSuccess: async (res) => {
        const url = res?.url ? window.location.origin + res.url : "";
        if (!url) {
          toast.error(t("media.library.shareFailed"));
          return;
        }
        try {
          await navigator.clipboard.writeText(url);
          toast.success(t("media.library.shareLinkCopied"));
        } catch {
          // Clipboard blocked (no secure context / permission) - surface the URL.
          toast.info(url);
        }
      },
      onError: (err: Error) => toast.error(err?.message || t("media.library.shareFailed")),
    },
  });

  return (
    <Button
      variant="secondary"
      loading={createLink.isPending}
      onClick={() => createLink.mutate({ mediaItemId: mediaId, data: { durationHours: 48 } })}
    >
      <Share2 size={16} />
      {t("media.library.shareLink")}
    </Button>
  );
}
