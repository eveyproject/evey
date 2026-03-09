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
    <div
      className="min-h-screen flex items-center justify-center px-4"
      style={{
        background: "linear-gradient(135deg, var(--bg) 0%, #1a1a2e 50%, var(--bg) 100%)",
      }}
    >
      <Card className="w-full max-w-md p-8 shadow-2xl">
        <div className="flex flex-col items-center mb-8">
          <div
            className="w-12 h-12 rounded-[var(--radius-md)] flex items-center justify-center mb-4"
            style={{ backgroundColor: "var(--accent)" }}
          >
            <svg className="w-6 h-6 text-white" fill="currentColor" viewBox="0 0 20 20">
              <path d="M6.3 2.8L17 10 6.3 17.2V2.8z" />
            </svg>
          </div>
          <h1 className="text-2xl font-bold text-[var(--text)]">Welcome to Evey</h1>
          <p className="text-[var(--text-muted)] text-sm mt-1">Set up your server to get started</p>
        </div>
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
