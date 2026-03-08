import { useState } from "react";
import { createFileRoute, isRedirect, redirect, useNavigate } from "@tanstack/react-router";
import { eveyFetch, useAdminServiceSetupWizard } from "@evey/api";
import { Button } from "@/components/ui/Button";
import { Input } from "@/components/ui/Input";
import { Card } from "@/components/ui/Card";

type SystemInfo = { setupComplete?: boolean };

export const Route = createFileRoute("/setup")({
  beforeLoad: async ({ context }) => {
    try {
      const info = await context.queryClient.fetchQuery<SystemInfo>({
        queryKey: ["system-info"],
        queryFn: () => eveyFetch<SystemInfo>({ url: "/api/v1/admin/system", method: "GET" }),
        staleTime: 30_000,
      });
      if (info.setupComplete) throw redirect({ to: "/login" });
    } catch (e) {
      if (isRedirect(e)) throw e;
      // backend unreachable — show setup page
    }
  },
  component: SetupPage,
});

interface SetupForm {
  adminUsername: string;
  adminPassword: string;
  adminEmail: string;
  serverName: string;
  baseUrl: string;
}

function SetupPage() {
  const navigate = useNavigate();
  const [form, setForm] = useState<SetupForm>({
    adminUsername: "",
    adminPassword: "",
    adminEmail: "",
    serverName: "My Evey Server",
    baseUrl: window.location.origin,
  });

  const setup = useAdminServiceSetupWizard({
    mutation: {
      onSuccess: () => {
        void navigate({ to: "/login" });
      },
      onError: (err: Error & { httpStatus?: number }) => {
        if (err?.httpStatus === 409) void navigate({ to: "/login" });
      },
    },
  });

  const update =
    (field: keyof SetupForm) => (e: React.ChangeEvent<HTMLInputElement>) =>
      setForm((prev) => ({ ...prev, [field]: e.target.value }));

  return (
    <div className="min-h-screen flex items-center justify-center bg-[var(--bg)]">
      <Card className="w-full max-w-md">
        <h1 className="text-2xl font-bold text-[var(--text)] mb-1">Welcome to Evey</h1>
        <p className="text-[var(--text-muted)] text-sm mb-6">Set up your server to get started</p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            setup.mutate({
              data: {
                adminUsername: form.adminUsername,
                adminPassword: form.adminPassword,
                adminEmail: form.adminEmail,
                serverName: form.serverName,
                baseUrl: form.baseUrl,
              },
            });
          }}
          className="flex flex-col gap-4"
        >
          <Input label="Admin username" value={form.adminUsername} onChange={update("adminUsername")} required />
          <Input
            label="Admin password"
            type="password"
            value={form.adminPassword}
            onChange={update("adminPassword")}
            required
          />
          <Input
            label="Admin email"
            type="email"
            value={form.adminEmail}
            onChange={update("adminEmail")}
            required
          />
          <Input label="Server name" value={form.serverName} onChange={update("serverName")} required />
          <Input label="Base URL" value={form.baseUrl} onChange={update("baseUrl")} required />
          {setup.error && (
            <p className="text-sm text-red-400">
              {setup.error instanceof Error ? setup.error.message : "Setup failed"}
            </p>
          )}
          <Button type="submit" loading={setup.isPending} className="w-full mt-2">
            Complete setup
          </Button>
        </form>
      </Card>
    </div>
  );
}
