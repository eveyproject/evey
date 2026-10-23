import { useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslation } from "react-i18next";
import {
  useAdminPluginServiceAddTrustedPublisher,
  getAdminPluginServiceListTrustedPublishersQueryKey,
  getAdminPluginServiceListPluginsQueryKey,
  apiErrorMessage,
} from "@evey/api";
import { toast } from "@evey/store";
import { Badge } from "@/components/ui/Badge";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Modal } from "@/components/ui/Modal";
import { useDemoLockFor } from "@/hooks/useDemoMode";
import { type PluginSignature, shortFingerprint, signatureKind } from "./pluginSignature";

// The signature verdict on a plugin card, with the "Trust this key" action for
// a valid signature whose key is not in the trusted-publisher list yet.
// canTrust is off in the install preview: the key is only known to the server once the package is installed.
export function PluginSignatureBadge({ signature, canTrust = true }: { signature?: PluginSignature; canTrust?: boolean }) {
  const { t } = useTranslation();
  const queryClient = useQueryClient();
  const { lock } = useDemoLockFor("/evey.v1.AdminPluginService/AddTrustedPublisher");
  const [asking, setAsking] = useState(false);
  const [name, setName] = useState("");
  const kind = signatureKind(signature);
  const fp = signature?.keyFingerprint ?? "";

  const addPublisher = useAdminPluginServiceAddTrustedPublisher({
    mutation: {
      onSuccess: () => {
        toast.success(t("admin.plugin.signature.trustedToast"));
        setAsking(false);
        setName("");
        void queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListTrustedPublishersQueryKey() });
        void queryClient.invalidateQueries({ queryKey: getAdminPluginServiceListPluginsQueryKey() });
      },
      onError: (err: Error) => toast.error(apiErrorMessage(err, t, t("admin.plugin.signature.trustFailed"))),
    },
  });

  return (
    <>
      {kind === "trusted" && (
        <Badge variant="success" title={fp} data-testid="plugin-signature">
          {t("admin.plugin.signature.signedBy", { name: signature?.publisher })}
        </Badge>
      )}
      {kind === "untrusted" && (
        <>
          <Badge variant="warning" title={fp} data-testid="plugin-signature">
            {t("admin.plugin.signature.signedUnknown", { fp: shortFingerprint(fp) })}
          </Badge>
          {canTrust && (
            <Button variant="ghost" onClick={() => setAsking(true)} data-testid="plugin-trust-key" {...lock}>
              {t("admin.plugin.signature.trustKey")}
            </Button>
          )}
        </>
      )}
      {kind === "unsigned" && (
        <Badge variant="default" data-testid="plugin-signature">{t("admin.plugin.signature.unsigned")}</Badge>
      )}
      {kind === "invalid" && (
        <Badge variant="error" title={signature?.error || fp} data-testid="plugin-signature">
          {t("admin.plugin.signature.invalid")}
        </Badge>
      )}

      <Modal open={asking} onClose={() => setAsking(false)} title={t("admin.plugin.signature.trustTitle")}>
        <div className="flex flex-col gap-3">
          <p className="text-xs font-mono break-all text-[var(--text-muted)]">{fp}</p>
          <Input
            label={t("admin.plugin.signature.trustNameLabel")}
            value={name}
            onChange={(e) => setName(e.target.value)}
          />
          <div className="flex justify-end gap-2">
            <Button variant="ghost" onClick={() => setAsking(false)}>{t("common.cancel")}</Button>
            <Button
              variant="primary"
              disabled={!name.trim()}
              loading={addPublisher.isPending}
              onClick={() => addPublisher.mutate({ data: { name: name.trim(), fingerprint: fp } })}
              data-testid="plugin-trust-confirm"
            >
              {t("admin.plugin.signature.trustConfirm")}
            </Button>
          </div>
        </div>
      </Modal>
    </>
  );
}
