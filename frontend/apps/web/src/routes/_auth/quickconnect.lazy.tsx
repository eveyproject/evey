import { useState, useEffect } from "react";
import { createLazyFileRoute, useNavigate } from "@tanstack/react-router";
import { useTranslation } from "react-i18next";
import { useUserServiceAuthorizeQuickConnect } from "@evey/api";
import { toast } from "@evey/store";
import { Card } from "@/components/ui/Card";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { QrCode, ArrowLeft } from "lucide-react";

export const Route = createLazyFileRoute("/_auth/quickconnect")({
  component: QuickConnectPage,
});

function QuickConnectPage() {
  const { t } = useTranslation();
  const navigate = useNavigate();
  const { code: urlCode } = Route.useSearch();
  const [code, setCode] = useState("");
  const [authorized, setAuthorized] = useState(false);

  useEffect(() => {
    if (urlCode) {
      const short = urlCode.slice(-6).toUpperCase();
      setCode(short);
    }
  }, [urlCode]);

  const authorize = useUserServiceAuthorizeQuickConnect({
    mutation: {
      onSuccess: () => {
        setAuthorized(true);
        toast.success(t("auth.settings.quickConnect.authorized"));
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
    if (urlCode && urlCode.length > 10) {
      authorize.mutate({ data: { code: urlCode } });
    } else {
      authorize.mutate({ data: { code: trimmed } });
    }
  };

  if (authorized) {
    return (
      <div className="flex items-center justify-center py-20">
        <Card className="w-full max-w-sm p-8 text-center">
          <div className="flex flex-col items-center gap-4">
            <div
              className="w-12 h-12 rounded-full flex items-center justify-center"
              style={{ backgroundColor: "var(--color-success)", color: "white" }}
            >
              ✓
            </div>
            <h2 className="text-lg font-bold text-[var(--text)]">
              {t("auth.settings.quickConnect.authorized")}
            </h2>
            <Button variant="secondary" onClick={() => void navigate({ to: "/home" })}>
              {t("common.back")}
            </Button>
          </div>
        </Card>
      </div>
    );
  }

  return (
    <div className="flex items-center justify-center py-20">
      <Card className="w-full max-w-sm p-8">
        <div className="flex flex-col items-center gap-6">
          <div
            className="w-12 h-12 rounded-[var(--radius-md)] flex items-center justify-center"
            style={{ backgroundColor: "var(--accent)" }}
          >
            <QrCode size={24} className="text-[var(--filter-active-text)]" />
          </div>
          <div className="text-center">
            <h2 className="text-lg font-bold text-[var(--text)]">
              {t("auth.settings.quickConnect.title")}
            </h2>
            <p className="text-sm text-[var(--text-muted)] mt-1">
              {t("auth.settings.quickConnect.description")}
            </p>
          </div>
          <form onSubmit={handleSubmit} className="w-full flex flex-col gap-4">
            <Input
              label={t("auth.settings.quickConnect.codeLabel")}
              type="text"
              value={code}
              onChange={(e) => setCode(e.target.value.toUpperCase())}
              placeholder="A1B2C3"
              maxLength={6}
              autoComplete="off"
              autoFocus
              className="font-mono tracking-widest text-center text-lg"
            />
            <Button
              type="submit"
              variant="primary"
              loading={authorize.isPending}
              disabled={code.trim().length < 4}
              className="w-full"
            >
              {t("auth.settings.quickConnect.authorize")}
            </Button>
          </form>
          <button
            type="button"
            onClick={() => void navigate({ to: "/home" })}
            className="flex items-center gap-1 text-sm text-[var(--text-muted)] hover:text-[var(--text)]"
          >
            <ArrowLeft size={14} />
            {t("common.back")}
          </button>
        </div>
      </Card>
    </div>
  );
}
