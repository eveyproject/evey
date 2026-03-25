import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useUserServiceAuthorizeQuickConnect } from "@evey/api";
import { toast } from "@evey/store";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";

export function QuickConnectSettings() {
  const { t } = useTranslation();
  const [code, setCode] = useState("");

  const authorize = useUserServiceAuthorizeQuickConnect({
    mutation: {
      onSuccess: () => {
        toast.success(t("auth.settings.quickConnect.authorized"));
        setCode("");
      },
      onError: () => {
        toast.error(t("auth.settings.quickConnect.invalidCode"));
      },
    },
  });

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    const trimmed = code.trim().toUpperCase();
    if (!trimmed) return;
    authorize.mutate({ data: { code: trimmed } });
  };

  return (
    <div className="flex flex-col gap-6">
      <div>
        <h3 className="text-sm font-semibold text-[var(--text)]">
          {t("auth.settings.quickConnect.title")}
        </h3>
        <p className="text-xs text-[var(--text-muted)] mt-1">
          {t("auth.settings.quickConnect.description")}
        </p>
      </div>
      <form onSubmit={handleSubmit} className="flex flex-col gap-3">
        <Input
          label={t("auth.settings.quickConnect.codeLabel")}
          type="text"
          value={code}
          onChange={(e) => setCode(e.target.value.toUpperCase())}
          placeholder="A1B2C3"
          maxLength={6}
          autoComplete="off"
          className="font-mono tracking-widest"
        />
        <div>
          <Button
            type="submit"
            variant="primary"
            loading={authorize.isPending}
            disabled={code.trim().length < 4}
          >
            {t("auth.settings.quickConnect.authorize")}
          </Button>
        </div>
      </form>
    </div>
  );
}
